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
	"github.com/blocktree/go-owaddress"
	"github.com/blocktree/openwallet/v2/openwallet"
	"strings"

	"github.com/blocktree/go-owcdrivers/addressEncoder"
)

var (
	alphabet = addressEncoder.BTCAlphabet
)

var (

	//QTUM stuff
	QTUM_mainnetAddressP2PKH         = addressEncoder.AddressType{"base58", alphabet, "doubleSHA256", "h160", 20, []byte{0x3A}, nil}
	QTUM_mainnetAddressP2SH          = addressEncoder.AddressType{"base58", alphabet, "doubleSHA256", "h160", 20, []byte{0x32}, nil}
	QTUM_mainnetPrivateWIF           = addressEncoder.AddressType{"base58", alphabet, "doubleSHA256", "", 32, []byte{0x80}, nil}
	QTUM_mainnetPrivateWIFCompressed = addressEncoder.AddressType{"base58", alphabet, "doubleSHA256", "", 32, []byte{0x80}, []byte{0x01}}
	QTUM_mainnetPublicBIP32          = addressEncoder.AddressType{"base58", alphabet, "doubleSHA256", "", 74, []byte{0x04, 0x88, 0xB2, 0x1E}, nil}
	QTUM_mainnetPrivateBIP32         = addressEncoder.AddressType{"base58", alphabet, "doubleSHA256", "", 74, []byte{0x04, 0x88, 0xAD, 0xE4}, nil}
	QTUM_testnetAddressP2PKH         = addressEncoder.AddressType{"base58", alphabet, "doubleSHA256", "h160", 20, []byte{0x78}, nil}
	QTUM_testnetAddressP2SH          = addressEncoder.AddressType{"base58", alphabet, "doubleSHA256", "h160", 20, []byte{0x6E}, nil}
	QTUM_testnetPrivateWIF           = addressEncoder.AddressType{"base58", alphabet, "doubleSHA256", "", 32, []byte{0xEF}, nil}
	QTUM_testnetPrivateWIFCompressed = addressEncoder.AddressType{"base58", alphabet, "doubleSHA256", "", 32, []byte{0xEF}, []byte{0x01}}
	QTUM_testnetPublicBIP32          = addressEncoder.AddressType{"base58", alphabet, "doubleSHA256", "", 74, []byte{0x04, 0x35, 0x87, 0xCF}, nil}
	QTUM_testnetPrivateBIP32         = addressEncoder.AddressType{"base58", alphabet, "doubleSHA256", "", 74, []byte{0x04, 0x35, 0x83, 0x94}, nil}
)

//AddressDecoderV2
type AddressDecoderV2 struct {
	*openwallet.AddressDecoderV2Base
	wm        *WalletManager
	IsTestNet bool
}

//NewAddressDecoder 地址解析器
func NewAddressDecoder(wm *WalletManager) *AddressDecoderV2 {
	decoder := AddressDecoderV2{}
	decoder.wm = wm
	return &decoder
}

//AddressDecode 地址解析
func (dec *AddressDecoderV2) AddressDecode(addr string, opts ...interface{}) ([]byte, error) {

	cfg := QTUM_mainnetAddressP2PKH
	if dec.IsTestNet {
		cfg = QTUM_testnetAddressP2PKH
	}

	if len(opts) > 0 {
		for _, opt := range opts {
			if at, ok := opt.(addressEncoder.AddressType); ok {
				cfg = at
			}
		}
	}

	return addressEncoder.AddressDecode(addr, cfg)
}

//AddressEncode 地址编码
func (dec *AddressDecoderV2) AddressEncode(hash []byte, opts ...interface{}) (string, error) {

	cfg := QTUM_mainnetAddressP2PKH
	if dec.IsTestNet {
		cfg = QTUM_testnetAddressP2PKH
	}

	if len(opts) > 0 {
		for _, opt := range opts {
			if at, ok := opt.(addressEncoder.AddressType); ok {
				cfg = at
			}
		}
	}

	address := addressEncoder.AddressEncode(hash, cfg)

	if dec.wm.Config.RPCServerType == RPCServerCore {
		//如果使用core钱包作为全节点，需要导入地址到core，这样才能查询地址余额和utxo
		err := dec.wm.ImportAddress(address, "")
		if err != nil {
			return "", err
		}
	}

	return address, nil
}

// AddressVerify 地址校验
func (dec *AddressDecoderV2) AddressVerify(address string, opts ...interface{}) bool {
	valid, err := owaddress.Verify("qtum", address)
	if err != nil {
		return false
	}
	return valid
}

//HashAddressToBaseAddress 哈希地址转编码地址
func HashAddressToBaseAddress(token string, isTestnet bool) string {
	token = strings.TrimPrefix(token, "0x")
	cfg := addressEncoder.QTUM_mainnetAddressP2PKH
	if isTestnet {
		cfg = addressEncoder.QTUM_testnetAddressP2PKH
	}

	hash, err := hex.DecodeString(token)
	if err != nil {
		return ""
	}
	if len(hash) == 0 {
		return ""
	}
	tokenAddressBase := addressEncoder.AddressEncode(hash, cfg)

	return tokenAddressBase

}
