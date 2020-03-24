package qtum_addrdec

import (
	"encoding/hex"
	"testing"
)

func TestAddressDecoder_AddressEncode(t *testing.T) {
	Default.IsTestNet = false

	p2pk, _ := hex.DecodeString("d3e7f1c96a7be7903867a17f18e16cae8fad8d4d")
	p2pkAddr, _ := Default.AddressEncode(p2pk)
	t.Logf("p2pkAddr: %s", p2pkAddr)

	p2sh, _ := hex.DecodeString("1406b6c5e35c62b425c627369edcc615c5089ccc")
	p2shAddr, _ := Default.AddressEncode(p2sh, QTUM_mainnetAddressP2SH)
	t.Logf("p2shAddr: %s", p2shAddr)
}

func TestAddressDecoder_AddressDecode(t *testing.T) {

	Default.IsTestNet = false

	p2pkAddr := "QfvSYZSMdtr4M6ShjPa2DhbdgYHkjegV6R"
	p2pkHash, _ := Default.AddressDecode(p2pkAddr)
	t.Logf("p2pkHash: %s", hex.EncodeToString(p2pkHash))

	p2shAddr := "M9j3nm5HAQ88bWbGYLk8YbVPsZJvvrLVVj"

	p2shHash, _ := Default.AddressDecode(p2shAddr, QTUM_mainnetAddressP2SH)
	t.Logf("p2shHash: %s", hex.EncodeToString(p2shHash))
}
