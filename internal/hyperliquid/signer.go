package hyperliquid

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/vmihailenco/msgpack/v5"
)

type Signer struct {
	privateKey *ecdsa.PrivateKey
	address    common.Address
	chainID    *big.Int
	isAgent    bool
	source     common.Address
}

func NewSigner(privateKeyHex string, network Network) (*Signer, error) {
	key, err := PrivateKeyFromHex(privateKeyHex)
	if err != nil {
		return nil, err
	}

	return &Signer{
		privateKey: key,
		address:    crypto.PubkeyToAddress(key.PublicKey),
		chainID:    big.NewInt(network.GetChainID()),
		isAgent:    false,
	}, nil
}

func NewAgentSigner(privateKeyHex string, sourceAddress string, network Network) (*Signer, error) {
	key, err := PrivateKeyFromHex(privateKeyHex)
	if err != nil {
		return nil, err
	}

	return &Signer{
		privateKey: key,
		address:    crypto.PubkeyToAddress(key.PublicKey),
		chainID:    big.NewInt(network.GetChainID()),
		isAgent:    true,
		source:     common.HexToAddress(sourceAddress),
	}, nil
}

func (s *Signer) Address() string {
	return s.address.Hex()
}

func (s *Signer) SourceAddress() string {
	if s.isAgent {
		return s.source.Hex()
	}
	return s.address.Hex()
}

func (s *Signer) IsAgent() bool {
	return s.isAgent
}

func (s *Signer) isMainnet() bool {
	return s.chainID.Int64() == MainnetChainID
}

func (s *Signer) SignAction(action any, nonce int64, vaultAddress string) (*Signature, error) {
	connectionID, err := s.computeActionHash(action, nonce, vaultAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to compute action hash: %w", err)
	}

	phantomSource := "b" // testnet
	if s.isMainnet() {
		phantomSource = "a" // mainnet
	}

	typedData := apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": []apitypes.Type{
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
			"Agent": []apitypes.Type{
				{Name: "source", Type: "string"},
				{Name: "connectionId", Type: "bytes32"},
			},
		},
		PrimaryType: "Agent",
		Domain: apitypes.TypedDataDomain{
			Name:              "Exchange",
			Version:           "1",
			ChainId:           math.NewHexOrDecimal256(1337), // Always 1337 for phantom agent
			VerifyingContract: "0x0000000000000000000000000000000000000000",
		},
		Message: apitypes.TypedDataMessage{
			"source":       phantomSource,
			"connectionId": connectionID,
		},
	}

	hash, err := hashTypedData(typedData)
	if err != nil {
		return nil, fmt.Errorf("failed to hash typed data: %w", err)
	}

	sig, err := crypto.Sign(hash, s.privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	return &Signature{
		R: "0x" + hex.EncodeToString(sig[:32]),
		S: "0x" + hex.EncodeToString(sig[32:64]),
		V: int(sig[64]) + 27,
	}, nil
}

// computeActionHash computes the connectionId for phantom agent signing:
//
//	keccak256(msgpack(action) || nonce_8bytes_bigendian || vault_flag [|| vault_addr])
func (s *Signer) computeActionHash(action any, nonce int64, vaultAddress string) ([]byte, error) {
	msgpackData, err := marshalActionMsgpack(action)
	if err != nil {
		return nil, fmt.Errorf("failed to msgpack encode action: %w", err)
	}

	var buf bytes.Buffer
	buf.Write(msgpackData)

	nonceBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(nonceBytes, uint64(nonce))
	buf.Write(nonceBytes)

	if vaultAddress != "" {
		buf.WriteByte(0x01)
		buf.Write(common.HexToAddress(vaultAddress).Bytes()) // 20 bytes
	} else {
		buf.WriteByte(0x00)
	}

	return crypto.Keccak256(buf.Bytes()), nil
}

func marshalActionMsgpack(action any) ([]byte, error) {
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	enc.SetCustomStructTag("json")
	if err := enc.Encode(action); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func hashTypedData(typedData apitypes.TypedData) ([]byte, error) {
	domainSeparator, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	if err != nil {
		return nil, fmt.Errorf("failed to hash domain: %w", err)
	}

	messageHash, err := typedData.HashStruct(typedData.PrimaryType, typedData.Message)
	if err != nil {
		return nil, fmt.Errorf("failed to hash message: %w", err)
	}

	// keccak256("\x19\x01" || domainSeparator || messageHash)
	rawData := []byte{0x19, 0x01}
	rawData = append(rawData, domainSeparator...)
	rawData = append(rawData, messageHash...)

	return crypto.Keccak256(rawData), nil
}

func PrivateKeyFromHex(hexKey string) (*ecdsa.PrivateKey, error) {
	hexKey = strings.TrimPrefix(hexKey, "0x")

	keyBytes, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("invalid hex key: %w", err)
	}

	if len(keyBytes) != 32 {
		return nil, fmt.Errorf("private key must be 32 bytes, got %d", len(keyBytes))
	}

	key, err := crypto.ToECDSA(keyBytes)
	// Best-effort wipe of the input slice. The returned key still holds
	// material in its big.Int, so this isn't a full secure erase.
	for i := range keyBytes {
		keyBytes[i] = 0
	}
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}
	return key, nil
}

func AddressFromPrivateKey(key *ecdsa.PrivateKey) string {
	return crypto.PubkeyToAddress(key.PublicKey).Hex()
}

func GeneratePrivateKey() (*ecdsa.PrivateKey, error) {
	return crypto.GenerateKey()
}

func PrivateKeyToHex(key *ecdsa.PrivateKey) string {
	return hex.EncodeToString(crypto.FromECDSA(key))
}
