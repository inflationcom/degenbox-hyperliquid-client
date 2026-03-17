package unit

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/inflationcom/degenbox-hyperliquid-client/internal/hyperliquid"
)

const (
	testPrivateKey = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	testAddress    = "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"

	agentPrivateKey = "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
	agentAddress    = "0x70997970C51812dc3A010C7d01b50e0d17dc79C8"
)

func TestPrivateKeyFromHex(t *testing.T) {
	t.Run("valid key with 0x prefix", func(t *testing.T) {
		key, err := hyperliquid.PrivateKeyFromHex("0x" + testPrivateKey)
		require.NoError(t, err)
		assert.NotNil(t, key)
	})

	t.Run("valid key without prefix", func(t *testing.T) {
		key, err := hyperliquid.PrivateKeyFromHex(testPrivateKey)
		require.NoError(t, err)
		assert.NotNil(t, key)
	})

	t.Run("invalid hex", func(t *testing.T) {
		_, err := hyperliquid.PrivateKeyFromHex("not-valid-hex")
		assert.Error(t, err)
	})

	t.Run("wrong length", func(t *testing.T) {
		_, err := hyperliquid.PrivateKeyFromHex("abcd1234")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "32 bytes")
	})

	t.Run("empty key", func(t *testing.T) {
		_, err := hyperliquid.PrivateKeyFromHex("")
		assert.Error(t, err)
	})
}

func TestAddressFromPrivateKey(t *testing.T) {
	key, err := hyperliquid.PrivateKeyFromHex(testPrivateKey)
	require.NoError(t, err)

	address := hyperliquid.AddressFromPrivateKey(key)
	assert.Equal(t, testAddress, address)
}

func TestGeneratePrivateKey(t *testing.T) {
	key, err := hyperliquid.GeneratePrivateKey()
	require.NoError(t, err)

	hexStr := hyperliquid.PrivateKeyToHex(key)
	assert.Len(t, hexStr, 64)

	addr := hyperliquid.AddressFromPrivateKey(key)
	assert.True(t, strings.HasPrefix(addr, "0x"))
	assert.Len(t, addr, 42)
}

func TestPrivateKeyToHex(t *testing.T) {
	key, err := hyperliquid.PrivateKeyFromHex(testPrivateKey)
	require.NoError(t, err)

	hexStr := hyperliquid.PrivateKeyToHex(key)
	assert.Equal(t, testPrivateKey, hexStr)
}

func TestNewSigner(t *testing.T) {
	t.Run("mainnet signer", func(t *testing.T) {
		signer, err := hyperliquid.NewSigner(testPrivateKey, hyperliquid.Mainnet)
		require.NoError(t, err)
		assert.NotNil(t, signer)
		assert.Equal(t, testAddress, signer.Address())
		assert.False(t, signer.IsAgent())
		assert.Equal(t, testAddress, signer.SourceAddress())
	})

	t.Run("testnet signer", func(t *testing.T) {
		signer, err := hyperliquid.NewSigner(testPrivateKey, hyperliquid.Testnet)
		require.NoError(t, err)
		assert.NotNil(t, signer)
		assert.Equal(t, testAddress, signer.Address())
	})

	t.Run("invalid key", func(t *testing.T) {
		_, err := hyperliquid.NewSigner("invalid", hyperliquid.Mainnet)
		assert.Error(t, err)
	})
}

func TestNewAgentSigner(t *testing.T) {
	t.Run("valid agent signer", func(t *testing.T) {
		signer, err := hyperliquid.NewAgentSigner(agentPrivateKey, testAddress, hyperliquid.Mainnet)
		require.NoError(t, err)
		assert.NotNil(t, signer)
		assert.Equal(t, agentAddress, signer.Address())
		assert.True(t, signer.IsAgent())
		assert.Equal(t, testAddress, signer.SourceAddress())
	})

	t.Run("invalid key", func(t *testing.T) {
		_, err := hyperliquid.NewAgentSigner("invalid", testAddress, hyperliquid.Mainnet)
		assert.Error(t, err)
	})
}

func TestSignAction(t *testing.T) {
	signer, err := hyperliquid.NewSigner(testPrivateKey, hyperliquid.Mainnet)
	require.NoError(t, err)

	t.Run("sign order action", func(t *testing.T) {
		action := hyperliquid.OrderAction{
			Type: "order",
			Orders: []hyperliquid.OrderWire{
				{
					A: 0,
					B: true,
					P: "45000",
					S: "0.1",
					R: false,
					T: hyperliquid.OrderType{
						Limit: &hyperliquid.LimitSpec{Tif: "Gtc"},
					},
				},
			},
			Grouping: hyperliquid.GroupingNA,
		}

		sig, err := signer.SignAction(action, 1678886400000, "")
		require.NoError(t, err)
		assert.NotNil(t, sig)

		assert.True(t, strings.HasPrefix(sig.R, "0x"))
		assert.True(t, strings.HasPrefix(sig.S, "0x"))
		assert.True(t, sig.V == 27 || sig.V == 28)
		assert.Len(t, sig.R, 66)
		assert.Len(t, sig.S, 66)
	})

	t.Run("sign cancel action", func(t *testing.T) {
		action := hyperliquid.CancelAction{
			Type: "cancel",
			Cancels: []hyperliquid.CancelSpec{
				{A: 0, O: 123456789},
			},
		}

		sig, err := signer.SignAction(action, 1678886400000, "")
		require.NoError(t, err)
		assert.NotNil(t, sig)
		assert.True(t, sig.V == 27 || sig.V == 28)
	})

	t.Run("sign with vault address", func(t *testing.T) {
		action := hyperliquid.OrderAction{
			Type:     "order",
			Orders:   []hyperliquid.OrderWire{},
			Grouping: hyperliquid.GroupingNA,
		}

		vaultAddr := "0x1234567890123456789012345678901234567890"
		sig, err := signer.SignAction(action, 1678886400000, vaultAddr)
		require.NoError(t, err)
		assert.NotNil(t, sig)
	})

	t.Run("deterministic signatures", func(t *testing.T) {
		action := hyperliquid.OrderAction{
			Type:     "order",
			Orders:   []hyperliquid.OrderWire{},
			Grouping: hyperliquid.GroupingNA,
		}

		nonce := int64(1678886400000)

		sig1, err := signer.SignAction(action, nonce, "")
		require.NoError(t, err)

		sig2, err := signer.SignAction(action, nonce, "")
		require.NoError(t, err)

		assert.Equal(t, sig1.R, sig2.R)
		assert.Equal(t, sig1.S, sig2.S)
		assert.Equal(t, sig1.V, sig2.V)
	})

	t.Run("different nonce produces different signature", func(t *testing.T) {
		action := hyperliquid.OrderAction{
			Type:     "order",
			Orders:   []hyperliquid.OrderWire{},
			Grouping: hyperliquid.GroupingNA,
		}

		sig1, err := signer.SignAction(action, 1678886400000, "")
		require.NoError(t, err)

		sig2, err := signer.SignAction(action, 1678886400001, "")
		require.NoError(t, err)

		assert.NotEqual(t, sig1.R, sig2.R)
	})
}

func TestSignAgentAction(t *testing.T) {
	agentSigner, err := hyperliquid.NewAgentSigner(agentPrivateKey, testAddress, hyperliquid.Mainnet)
	require.NoError(t, err)

	t.Run("agent signs order", func(t *testing.T) {
		action := hyperliquid.OrderAction{
			Type: "order",
			Orders: []hyperliquid.OrderWire{
				{
					A: 0,
					B: true,
					P: "45000",
					S: "0.1",
					R: false,
					T: hyperliquid.OrderType{
						Limit: &hyperliquid.LimitSpec{Tif: "Gtc"},
					},
				},
			},
			Grouping: hyperliquid.GroupingNA,
		}

		sig, err := agentSigner.SignAction(action, 1678886400000, "")
		require.NoError(t, err)
		assert.NotNil(t, sig)
		assert.True(t, sig.V == 27 || sig.V == 28)
	})

	t.Run("agent signature differs from direct", func(t *testing.T) {
		directSigner, err := hyperliquid.NewSigner(testPrivateKey, hyperliquid.Mainnet)
		require.NoError(t, err)

		action := hyperliquid.OrderAction{
			Type:     "order",
			Orders:   []hyperliquid.OrderWire{},
			Grouping: hyperliquid.GroupingNA,
		}

		nonce := int64(1678886400000)

		directSig, err := directSigner.SignAction(action, nonce, "")
		require.NoError(t, err)

		agentSig, err := agentSigner.SignAction(action, nonce, "")
		require.NoError(t, err)

		assert.NotEqual(t, directSig.R, agentSig.R)
	})
}

func TestSignatureFormat(t *testing.T) {
	signer, err := hyperliquid.NewSigner(testPrivateKey, hyperliquid.Mainnet)
	require.NoError(t, err)

	action := hyperliquid.OrderAction{
		Type:     "order",
		Orders:   []hyperliquid.OrderWire{},
		Grouping: hyperliquid.GroupingNA,
	}

	sig, err := signer.SignAction(action, 1678886400000, "")
	require.NoError(t, err)

	_, err = hex.DecodeString(strings.TrimPrefix(sig.R, "0x"))
	assert.NoError(t, err)
	assert.Len(t, sig.R, 66)

	_, err = hex.DecodeString(strings.TrimPrefix(sig.S, "0x"))
	assert.NoError(t, err)
	assert.Len(t, sig.S, 66)

	assert.True(t, sig.V == 27 || sig.V == 28)
}

func TestNetworkSpecificSigning(t *testing.T) {
	action := hyperliquid.OrderAction{
		Type:     "order",
		Orders:   []hyperliquid.OrderWire{},
		Grouping: hyperliquid.GroupingNA,
	}

	nonce := int64(1678886400000)

	t.Run("mainnet vs testnet produce different signatures", func(t *testing.T) {
		mainnetSigner, err := hyperliquid.NewSigner(testPrivateKey, hyperliquid.Mainnet)
		require.NoError(t, err)

		testnetSigner, err := hyperliquid.NewSigner(testPrivateKey, hyperliquid.Testnet)
		require.NoError(t, err)

		mainnetSig, err := mainnetSigner.SignAction(action, nonce, "")
		require.NoError(t, err)

		testnetSig, err := testnetSigner.SignAction(action, nonce, "")
		require.NoError(t, err)

		assert.NotEqual(t, mainnetSig.R, testnetSig.R,
			"Mainnet and testnet signatures should differ due to chainId")
	})
}

func TestSignComplexActions(t *testing.T) {
	signer, err := hyperliquid.NewSigner(testPrivateKey, hyperliquid.Mainnet)
	require.NoError(t, err)

	t.Run("batch order with TP/SL", func(t *testing.T) {
		action := hyperliquid.OrderAction{
			Type: "order",
			Orders: []hyperliquid.OrderWire{
				{
					A: 0,
					B: true,
					P: "45000",
					S: "0.1",
					R: false,
					T: hyperliquid.OrderType{
						Limit: &hyperliquid.LimitSpec{Tif: "Gtc"},
					},
					C: "0x0001",
				},
				{
					A: 0,
					B: false,
					P: "50000",
					S: "0.1",
					R: true,
					T: hyperliquid.OrderType{
						Trigger: &hyperliquid.TriggerWire{
							IsMarket:  true,
							TriggerPx: "50000",
							TpSl:      "tp",
						},
					},
					C: "0x0002",
				},
				{
					A: 0,
					B: false,
					P: "43000",
					S: "0.1",
					R: true,
					T: hyperliquid.OrderType{
						Trigger: &hyperliquid.TriggerWire{
							IsMarket:  true,
							TriggerPx: "43000",
							TpSl:      "sl",
						},
					},
					C: "0x0003",
				},
			},
			Grouping: hyperliquid.GroupingNormalTpsl,
		}

		sig, err := signer.SignAction(action, 1678886400000, "")
		require.NoError(t, err)
		assert.NotNil(t, sig)
	})

	t.Run("modify action", func(t *testing.T) {
		action := hyperliquid.ModifyAction{
			Type: "batchModify",
			Modifies: []hyperliquid.ModifySpec{
				{
					Oid: 123456789,
					Order: hyperliquid.OrderWire{
						A: 0,
						B: true,
						P: "46000",
						S: "0.2",
						R: false,
						T: hyperliquid.OrderType{
							Limit: &hyperliquid.LimitSpec{Tif: "Gtc"},
						},
					},
				},
			},
		}

		sig, err := signer.SignAction(action, 1678886400000, "")
		require.NoError(t, err)
		assert.NotNil(t, sig)
	})

	t.Run("cancel by cloid", func(t *testing.T) {
		action := hyperliquid.CancelByCloidAction{
			Type: "cancelByCloid",
			Cancels: []hyperliquid.CancelCloidSpec{
				{Asset: 0, Cloid: "0x0001"},
				{Asset: 0, Cloid: "0x0002"},
			},
		}

		sig, err := signer.SignAction(action, 1678886400000, "")
		require.NoError(t, err)
		assert.NotNil(t, sig)
	})

	t.Run("update leverage", func(t *testing.T) {
		action := hyperliquid.UpdateLeverageAction{
			Type:     "updateLeverage",
			Asset:    0,
			IsCross:  true,
			Leverage: 20,
		}

		sig, err := signer.SignAction(action, 1678886400000, "")
		require.NoError(t, err)
		assert.NotNil(t, sig)
	})
}
