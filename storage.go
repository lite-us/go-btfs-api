package shell

import (
	"context"
	"errors"
	"fmt"
	"github.com/gogo/protobuf/proto"
	"github.com/tron-us/go-common/v2/json"
	"strconv"
	"time"

	utils "github.com/TRON-US/go-btfs-api/utils"

	"github.com/tron-us/go-btfs-common/crypto"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	ledgerpb "github.com/tron-us/go-btfs-common/protos/ledger"
	"github.com/tron-us/go-common/v2/log"

	"go.uber.org/zap"
)

type StorageUploadOpts = func(*RequestBuilder) error

type storageUploadResponse struct {
	ID string
}

type shard struct {
	ContractId string
	Price      int64
	Host       string
	Status     string
}

type Storage struct {
	Status   string
	Message  string
	FileHash string
	Shards   map[string]shard
}

type ContractItem struct {
	Key string         `json:"key"`
	Contract   string     `json:"contract"`
}

type Contracts struct {
	Contracts []ContractItem `json:contracts`
}

type UnsignedData struct {
	Unsigned string
	Opcode   string
	Price 	int64
}

type StorageOpts = func(*RequestBuilder) error

func OfflineSignMode(enabled bool) StorageOpts {
	return func(rb *RequestBuilder) error {
		rb.Option("offline-sign-mode", enabled)
		return nil
	}
}

func (d UnsignedData) SignData(privateKey string) ([]byte, error) {
	privKey, _ := crypto.ToPrivKey(privateKey)
	signedData, err := privKey.Sign([]byte(d.Unsigned))
	if err != nil {
		return nil, err
	}
	return signedData, nil
}

func (d UnsignedData) SignBalanceData(privateKey string) (*ledgerpb.SignedPublicKey, error) {
	privKey, _ := crypto.ToPrivKey(privateKey)
	pubKeyRaw, err := privKey.GetPublic().Raw()
	if err != nil {
		return &ledgerpb.SignedPublicKey{}, err
	}
	lgPubKey := &ledgerpb.PublicKey{
		Key: pubKeyRaw,
	}
	sig, err := crypto.Sign(privKey, lgPubKey)
	if err != nil {
		return &ledgerpb.SignedPublicKey{}, err
	}
	lgSignedPubKey := &ledgerpb.SignedPublicKey{
		Key:       lgPubKey,
		Signature: sig,
	}
	return lgSignedPubKey, nil
}
func (c Contracts) SignContracts(privateKey string) (*Contracts, error) {
	//do some signing here using private key
	privKey, err := crypto.ToPrivKey(privateKey)
	if err != nil {
		log.Error("%s", zap.Error(err))
	}
	for contractIndex, element := range c.Contracts {
		signedContract, err := privKey.Sign([]byte(element.Contract))
		c.Contracts[contractIndex].Contract = string(signedContract)
		if err != nil {
			return nil, err
		}
	}
	return &c, nil
}

// Set storage upload time.
func StorageLength(length int) StorageUploadOpts {
	return func(rb *RequestBuilder) error {
		rb.Option("storage-length", length)
		return nil
	}
}

func (s *Shell) GetUts() string {
	return strconv.FormatInt(time.Now().Unix(), 10)
}

func getSessionSignature(hash string, peerId string) (string, time.Time) {
	//offline session signature
	now := time.Now()
	sessionSignature := fmt.Sprintf("%s:%s:%s", utils.PeerId , hash ,"time.Now().String()")
	return sessionSignature, now
}

// Storage upload api.
func (s *Shell) StorageUpload(hash string) (string, error) {
	var out storageUploadResponse
	rb := s.Request("storage/upload", hash)
	return out.ID, rb.Exec(context.Background(), &out)
}

// Storage upload api.
func (s *Shell) StorageUploadOffSign(hash string, uts string, options ...StorageUploadOpts) (string, error) {
	var out storageUploadResponse
	offlinePeerSessionSignature, _ :=  getSessionSignature(hash, utils.PeerId)
	rb := s.Request("storage/upload", hash, utils.PeerId, uts, offlinePeerSessionSignature)
	for _, option := range options {
		_ = option(rb)
	}
	return out.ID, rb.Exec(context.Background(), &out)
}

// Storage upload status api.
func (s *Shell) StorageUploadStatus(id string) (Storage, error) {
	var out Storage
	rb := s.Request("storage/upload/status", id)
	return out, rb.Exec(context.Background(), &out)
}

// Storage upload get offline contract batch api.
func (s *Shell) StorageUploadGetContractBatch(sid string, hash string, uts string, sessionStatus string) (Contracts, error) {
	var out Contracts
	offlinePeerSessionSignature, _ :=  getSessionSignature(hash, utils.PeerId)
	rb := s.Request("storage/upload/getcontractbatch", sid, utils.PeerId, uts, offlinePeerSessionSignature, sessionStatus)
	return out, rb.Exec(context.Background(), &out)
}

// Storage upload get offline unsigned data api.
func (s *Shell) StorageUploadGetUnsignedData(id string, hash string, uts string, sessionStatus string) (UnsignedData, error) {
	var out UnsignedData
	offlinePeerSessionSignature, _ :=  getSessionSignature(hash, utils.PeerId)
	rb := s.Request("storage/upload/getunsigned", id, utils.PeerId, uts, offlinePeerSessionSignature, sessionStatus)
	return out, rb.Exec(context.Background(), &out)
}

// Storage upload sign offline contract batch api.
func (s *Shell) StorageUploadSignBatch(sid string, hash string, unsignedBatchContracts Contracts, uts string, sessionStatus string) ([]byte, error) {
	var out []byte
	var signedBatchContracts *Contracts
	var errSign error
	offlinePeerSessionSignature, _ :=  getSessionSignature(hash, utils.PeerId)

	if utils.PrivateKey != "" {
		signedBatchContracts, errSign = unsignedBatchContracts.SignContracts(utils.PrivateKey)
		if errSign != nil {
			log.Error("%s", zap.Error(errSign))
		}
		byteSignedBatchContracts, err := json.Marshal(signedBatchContracts.Contracts)
		if err != nil {
			return nil, err
		}
		rb := s.Request("storage/upload/signbatch", sid, utils.PeerId, uts, offlinePeerSessionSignature, sessionStatus, string(byteSignedBatchContracts))
		return out, rb.Exec(context.Background(), &out)
	}
	return nil, errors.New("private key not available in configuration file or environment variable")
}

// Storage upload sign offline data api.
func (s *Shell) StorageUploadSign(id string, hash string, unsignedData UnsignedData, uts string, sessionStatus string) ([]byte, error) {
	var out []byte
	var rb *RequestBuilder
	offlinePeerSessionSignature, _ :=  getSessionSignature(hash, utils.PeerId)
	if utils.PrivateKey != "" {
		signedBytes, err := unsignedData.SignData(utils.PrivateKey)
		if err != nil {
			log.Error("%s", zap.Error(err))
		}
		rb = s.Request("storage/upload/sign", id, utils.PeerId, uts, offlinePeerSessionSignature, string(signedBytes), sessionStatus)
		return out, rb.Exec(context.Background(), &out)
	}
	return nil, errors.New("private key not available in configuration file or environment variable")
}

func (s *Shell) StorageUploadSignBalance(id string,  hash string, unsignedData UnsignedData, uts string, sessionStatus string) ([]byte, error) {
	var out []byte
	var rb *RequestBuilder
	offlinePeerSessionSignature, _ :=  getSessionSignature(hash, utils.PeerId)
	if utils.PrivateKey != "" {
		ledgerSignedPublicKey, err := unsignedData.SignBalanceData(utils.PrivateKey)
		if err != nil {
			log.Error("%s", zap.Error(err))
		}
		rb = s.Request("storage/upload/sign", id, utils.PeerId, uts, offlinePeerSessionSignature, ledgerSignedPublicKey.String(), sessionStatus)
		return out, rb.Exec(context.Background(), &out)
	}
	return nil, errors.New("private key not available in configuration file or environment variable")
}

func (s *Shell) StorageUploadSignPayChannel(id, hash string, unsignedData UnsignedData, uts string, sessionStatus string, totalPrice int64) ([]byte, error) {
	var out []byte
	var rb *RequestBuilder
	offlinePeerSessionSignature, now :=  getSessionSignature(hash, utils.PeerId)
	if utils.PrivateKey != "" {
		chanCommit := &ledgerpb.ChannelCommit{
			Amount: totalPrice, PayerId:now.UnixNano(),
			Payer: &ledgerpb.PublicKey{Key:[]byte(utils.PublicKey)},
			Recipient:&ledgerpb.PublicKey{Key: []byte(unsignedData.Unsigned)},
		}
		privKey, _ := crypto.ToPrivKey(utils.PrivateKey)
		buyerChanSig, err := crypto.Sign(privKey, chanCommit)
		if err != nil {
			return nil, err
		}
		signedChanCommit := &ledgerpb.SignedChannelCommit{
			Channel:   chanCommit,
			Signature: buyerChanSig,
		}
		signedChanCommitBytes, err := proto.Marshal(signedChanCommit)
		if err != nil {
			return nil, err
		}
		rb = s.Request("storage/upload/sign", id, utils.PeerId, uts, offlinePeerSessionSignature, string(signedChanCommitBytes), sessionStatus)
		return out, rb.Exec(context.Background(), &out)
	}
	return nil, errors.New("private key not available in configuration file or environment variable")
}

func (s *Shell) StorageUploadSignPayRequest(id, hash string, unsignedData UnsignedData, uts string, sessionStatus string) ([]byte, error) {
	var out []byte
	var rb *RequestBuilder
	offlinePeerSessionSignature, _ :=  getSessionSignature(hash, utils.PeerId)
	if utils.PrivateKey != "" {
		result := &escrowpb.SignedSubmitContractResult{}
		err := proto.Unmarshal([]byte(unsignedData.Unsigned), result)
		if err != nil {
			return nil, err
		}

		chanState := result.Result.BuyerChannelState
		privKey, _ := crypto.ToPrivKey(utils.PrivateKey)
		sig, err := crypto.Sign(privKey, chanState)
		if err != nil {
			return nil, err
		}
		chanState.FromSignature = sig
		payerPubKey, _ := crypto.ToPrivKey(utils.PublicKey)
		payerAddr, err := payerPubKey.Raw()
		if err != nil {
			return nil, err
		}
		payinReq := &escrowpb.PayinRequest{
			PayinId:           result.Result.PayinId,
			BuyerAddress:      payerAddr,
			BuyerChannelState: chanState,
		}
		payinSig, err := crypto.Sign(privKey, payinReq)
		if err != nil {
			return nil, err
		}
		signedPayinReq := &escrowpb.SignedPayinRequest{
			Request:        payinReq,
			BuyerSignature: payinSig,
		}

		signedPayinReqBytes, err := proto.Marshal(signedPayinReq)
		if err != nil {
			return nil, err
		}

		rb = s.Request("storage/upload/sign", id, utils.PeerId, uts, offlinePeerSessionSignature, string(signedPayinReqBytes), sessionStatus)
		return out, rb.Exec(context.Background(), &out)
	}
	return nil, errors.New("private key not available in configuration file or environment variable")
}
