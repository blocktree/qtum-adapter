/*
 * Copyright 2018 The openwallet Authors
 * This file is part of the openwallet library.
 *
 * The openwallet library is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Lesser General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * The openwallet library is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 * GNU Lesser General Public License for more details.
 */

package qtum

import (
	"encoding/hex"
	"testing"
)

func TestAddressDecoder_AddressEncode(t *testing.T) {

	addrdec := tw.GetAddressDecoderV2()
	p2pk, _ := hex.DecodeString("d3e7f1c96a7be7903867a17f18e16cae8fad8d4d")
	p2pkAddr, _ := addrdec.AddressEncode(p2pk)
	t.Logf("p2pkAddr: %s", p2pkAddr)

	p2sh, _ := hex.DecodeString("1406b6c5e35c62b425c627369edcc615c5089ccc")
	p2shAddr, _ := addrdec.AddressEncode(p2sh, QTUM_mainnetAddressP2SH)
	t.Logf("p2shAddr: %s", p2shAddr)
}

func TestAddressDecoder_AddressDecode(t *testing.T) {

	addrdec := tw.GetAddressDecoderV2()
	p2pkAddr := "QfvSYZSMdtr4M6ShjPa2DhbdgYHkjegV6R"
	p2pkHash, _ := addrdec.AddressDecode(p2pkAddr)
	t.Logf("p2pkHash: %s", hex.EncodeToString(p2pkHash))

	p2shAddr := "M9j3nm5HAQ88bWbGYLk8YbVPsZJvvrLVVj"

	p2shHash, _ := addrdec.AddressDecode(p2shAddr, QTUM_mainnetAddressP2SH)
	t.Logf("p2shHash: %s", hex.EncodeToString(p2shHash))
}

func TestHashAddressToBaseAddress(t *testing.T) {
	addr := HashAddressToBaseAddress("", true)
	t.Logf("addr: %s", addr)
}
