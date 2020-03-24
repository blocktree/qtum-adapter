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
	"errors"
	"fmt"
	"github.com/blocktree/openwallet/v2/log"
	"github.com/blocktree/openwallet/v2/openwallet"
	"github.com/imroc/req"
	"github.com/shopspring/decimal"
	"github.com/tidwall/gjson"
	"net/http"
	"strings"
)

// Explorer是由bitpay的insight-API提供区块数据查询接口
// 具体接口说明查看https://github.com/bitpay/insight-api
type Explorer struct {
	BaseURL     string
	AccessToken string
	Debug       bool
	client      *req.Req
	//Client *req.Req
}

func NewExplorer(url string, debug bool) *Explorer {
	c := Explorer{
		BaseURL: url,
		//AccessToken: token,
		Debug: debug,
	}

	api := req.New()
	c.client = api

	return &c
}

// Call calls a remote procedure on another node, specified by the path.
func (b *Explorer) Call(path string, request interface{}, method string) (*gjson.Result, error) {

	if b.client == nil {
		return nil, errors.New("API url is not setup. ")
	}

	if b.Debug {
		log.Std.Debug("Start Request API...")
	}

	url := b.BaseURL + path

	r, err := b.client.Do(method, url, request)

	if b.Debug {
		log.Std.Debug("Request API Completed")
	}

	if b.Debug {
		log.Std.Debug("%+v", r)
	}

	err = b.isError(r)
	if err != nil {
		return nil, err
	}

	if err != nil {
		return nil, err
	}

	resp := gjson.ParseBytes(r.Bytes())

	return &resp, nil
}

//isError 是否报错
func (b *Explorer) isError(resp *req.Resp) error {

	if resp == nil || resp.Response() == nil {
		return errors.New("Response is empty! ")
	}

	if resp.Response().StatusCode != http.StatusOK {
		return fmt.Errorf("%s", resp.String())
	}

	return nil
}

//getBlockByExplorer 获取区块数据
func (wm *WalletManager) getBlockByExplorer(hash string) (*Block, error) {

	path := fmt.Sprintf("block/%s", hash)

	result, err := wm.ExplorerClient.Call(path, nil, "GET")
	if err != nil {
		return nil, err
	}

	return newBlockByExplorer(result), nil
}

//getBlockHashByExplorer 获取区块hash
func (wm *WalletManager) getBlockHashByExplorer(height uint64) (string, error) {

	path := fmt.Sprintf("block/%d", height)

	result, err := wm.ExplorerClient.Call(path, nil, "GET")
	if err != nil {
		return "", err
	}

	return result.Get("hash").String(), nil
}

//getBlockHeightByExplorer 获取区块链高度
func (wm *WalletManager) getBlockHeightByExplorer() (uint64, error) {

	path := "info"

	result, err := wm.ExplorerClient.Call(path, nil, "GET")
	if err != nil {
		return 0, err
	}

	height := result.Get("height").Uint()

	return height, nil
}

//getTxIDsInMemPoolByExplorer 获取待处理的交易池中的交易单IDs
func (wm *WalletManager) getTxIDsInMemPoolByExplorer() ([]string, error) {

	return nil, nil
}

//GetTransaction 获取交易单
func (wm *WalletManager) getTransactionByExplorer(txid string) (*Transaction, error) {

	path := fmt.Sprintf("tx/%s", txid)

	result, err := wm.ExplorerClient.Call(path, nil, "GET")
	if err != nil {
		return nil, err
	}

	tx := wm.newTxByExplorer(result, wm.Config.isTestNet)

	return tx, nil

}

//listUnspentByExplorer 获取未花交易
func (wm *WalletManager) listUnspentByExplorer(address ...string) ([]*Unspent, error) {

	var (
		utxos = make([]*Unspent, 0)
	)

	for _, addr := range address {

		path := fmt.Sprintf("address/%s/utxo", addr)

		result, err := wm.ExplorerClient.Call(path, nil, "GET")
		if err != nil {
			return nil, err
		}

		array := result.Array()
		for _, a := range array {
			utxos = append(utxos, wm.newUnspentByExplorer(&a))
		}

	}

	return utxos, nil

}

func (wm *WalletManager) newUnspentByExplorer(json *gjson.Result) *Unspent {
	obj := &Unspent{}
	//解析json
	obj.TxID = gjson.Get(json.Raw, "transactionId").String()
	obj.Vout = gjson.Get(json.Raw, "outputIndex").Uint()
	obj.Address = gjson.Get(json.Raw, "address").String()
	//obj.AccountID = gjson.Get(json.Raw, "account").String()
	obj.ScriptPubKey = gjson.Get(json.Raw, "scriptPubKey").String()
	amount, _ := decimal.NewFromString(gjson.Get(json.Raw, "value").String())
	obj.Amount = amount.Shift(-wm.Decimal()).String()
	obj.Confirmations = gjson.Get(json.Raw, "confirmations").Uint()
	isStake := gjson.Get(json.Raw, "isStake").Bool()
	if isStake {
		//挖矿的UTXO需要超过500个确认才能用
		if obj.Confirmations >= StakeConfirmations {
			obj.Spendable = true
		} else {
			obj.Spendable = false
		}
	} else {
		obj.Spendable = true
	}
	//obj.Solvable = gjson.Get(json.Raw, "solvable").Bool()

	return obj
}

func newBlockByExplorer(json *gjson.Result) *Block {

	/*
		{
			"hash": "0000000000002bd2475d1baea1de4067ebb528523a8046d5f9d8ef1cb60460d3",
			"size": 549,
			"height": 1434016,
			"version": 536870912,
			"merkleroot": "ae4310c991ec16cfc7404aaad9fe5fbd533d0b6617c03eb1ac644c89d58b3e18",
			"tx": ["6767a8acc1a63c7978186c582fdea26c47da5e04b0b2b34740a1728bfd959a05", "226dee96373aedd8a3dd00021684b190b7f23f5e16bb186cee11d0560406c19d"],
			"time": 1539066282,
			"nonce": 4089837546,
			"bits": "1a3fffc0",
			"difficulty": 262144,
			"chainwork": "0000000000000000000000000000000000000000000000c6fce84fddeb57e5fb",
			"confirmations": 279,
			"previousblockhash": "0000000000001fdabb5efc93d15ccaf6980642918cd898df6b3ff5fbf26c19c4",
			"nextblockhash": "00000000000024f2bd323157e595613291f83485ddfbbf311323ed0c0dc46545",
			"reward": 0.78125,
			"isMainChain": true,
			"poolInfo": {}
		}
	*/
	obj := &Block{}
	//解析json
	obj.Hash = gjson.Get(json.Raw, "hash").String()
	obj.Confirmations = gjson.Get(json.Raw, "confirmations").Uint()
	obj.Merkleroot = gjson.Get(json.Raw, "merkleRoot").String()

	txs := make([]string, 0)
	for _, tx := range gjson.Get(json.Raw, "transactions").Array() {
		txs = append(txs, tx.String())
	}

	obj.tx = txs
	obj.Previousblockhash = gjson.Get(json.Raw, "prevHash").String()
	obj.Height = gjson.Get(json.Raw, "height").Uint()
	//obj.Version = gjson.Get(json.Raw, "version").String()
	obj.Time = gjson.Get(json.Raw, "timestamp").Uint()

	return obj
}

func (wm *WalletManager) newTxByExplorer(json *gjson.Result, isTestnet bool) *Transaction {

	obj := Transaction{}
	//解析json
	obj.TxID = gjson.Get(json.Raw, "id").String()
	obj.Version = gjson.Get(json.Raw, "version").Uint()
	obj.LockTime = gjson.Get(json.Raw, "lockTime").Int()
	obj.BlockHash = gjson.Get(json.Raw, "blockHash").String()
	obj.BlockHeight = gjson.Get(json.Raw, "blockHeight").Uint()
	if obj.BlockHeight <= 0 {
		obj.BlockHeight = 0
	}
	//obj.BlockHeight = gjson.Get(json.Raw, "blockheight").Uint()
	obj.Confirmations = gjson.Get(json.Raw, "confirmations").Uint()
	obj.Blocktime = gjson.Get(json.Raw, "timestamp").Int()
	obj.Size = gjson.Get(json.Raw, "size").Uint()
	fees, _ := decimal.NewFromString(gjson.Get(json.Raw, "fees").String())
	obj.Fees = fees.Shift(-wm.Decimal()).String()
	obj.IsCoinBase = gjson.Get(json.Raw, "isCoinbase").Bool()
	obj.IsCoinstake = gjson.Get(json.Raw, "isCoinstake").Bool()

	obj.Vins = make([]*Vin, 0)
	if vins := gjson.Get(json.Raw, "inputs"); vins.IsArray() {
		for i, vin := range vins.Array() {
			input := wm.newTxVinByExplorer(&vin)
			input.N = uint64(i)
			obj.Vins = append(obj.Vins, input)
		}
	}

	obj.Vouts = make([]*Vout, 0)
	if vouts := gjson.Get(json.Raw, "outputs"); vouts.IsArray() {
		for i, vout := range vouts.Array() {
			output := wm.newTxVoutByExplorer(&vout)
			output.N = uint64(i)
			obj.Vouts = append(obj.Vouts, output)
		}
	}

	obj.TokenReceipts = make([]*TokenReceipt, 0)
	if receipts := gjson.Get(json.Raw, "qrc20TokenTransfers"); receipts.IsArray() {
		obj.Isqrc20Transfer = true
		for _, receipt := range receipts.Array() {
			token := newTokenReceiptByExplorer(&receipt, isTestnet)
			token.TxHash = obj.TxID
			token.BlockHash = obj.BlockHash
			token.BlockHeight = obj.BlockHeight
			obj.TokenReceipts = append(obj.TokenReceipts, token)
		}
	}

	return &obj
}

func (wm *WalletManager) newTxVinByExplorer(json *gjson.Result) *Vin {

	obj := Vin{}
	//解析json
	obj.TxID = gjson.Get(json.Raw, "prevTxId").String()
	obj.Vout = gjson.Get(json.Raw, "outputIndex").Uint()
	//obj.N = gjson.Get(json.Raw, "n").Uint()
	obj.Addr = gjson.Get(json.Raw, "address").String()
	value, _ := decimal.NewFromString(gjson.Get(json.Raw, "value").String())
	obj.Value = value.Shift(-wm.Decimal()).String()
	//obj.Coinbase = gjson.Get(json.Raw, "coinbase").String()

	return &obj
}

func (wm *WalletManager) newTxVoutByExplorer(json *gjson.Result) *Vout {

	obj := Vout{}
	//解析json
	value, _ := decimal.NewFromString(gjson.Get(json.Raw, "value").String())
	obj.Value = value.Shift(-wm.Decimal()).String()
	obj.N = gjson.Get(json.Raw, "n").Uint()
	obj.ScriptPubKey = gjson.Get(json.Raw, "scriptPubKey.hex").String()

	//提取地址
	obj.Addr = gjson.Get(json.Raw, "address").String()
	obj.Type = gjson.Get(json.Raw, "scriptPubKey.type").String()

	return &obj
}

//newTokenReceiptByExplorer
func newTokenReceiptByExplorer(json *gjson.Result, isTestnet bool) *TokenReceipt {

	obj := TokenReceipt{}
	//解析json

	obj.From = gjson.Get(json.Raw, "from").String()
	obj.To = gjson.Get(json.Raw, "to").String()
	obj.Amount = gjson.Get(json.Raw, "value").String()
	obj.ContractAddress = "0x" + gjson.Get(json.Raw, "addressHex").String()

	//obj.BlockHash = gjson.Get(json.Raw, "blockHash").String()
	//obj.BlockHeight = gjson.Get(json.Raw, "blockNumber").Uint()
	//obj.TxHash = gjson.Get(json.Raw, "transactionHash").String()
	//obj.Excepted = gjson.Get(json.Raw, "excepted").String()
	//obj.GasUsed = gjson.Get(json.Raw, "gasUsed").Uint()
	//obj.ContractAddress = "0x" + gjson.Get(json.Raw, "contractAddress").String()
	//obj.Sender = HashAddressToBaseAddress(
	//	gjson.Get(json.Raw, "from").String(),
	//	isTestnet)
	//
	//
	//
	//logs := gjson.Get(json.Raw, "log").Array()
	//for _, logInfo := range logs {
	//	topics := logInfo.Get("topics").Array()
	//	data := logInfo.Get("data").String()
	//
	//	if len(topics) != 3 {
	//		continue
	//	}
	//
	//	if "0x"+topics[0].String() != QTUM_TRANSFER_EVENT_ID {
	//		continue
	//	}
	//
	//	if len(data) != 64 {
	//		continue
	//	}
	//
	//	//log.Info("topics[1]:", topics[1].String())
	//	//log.Info("topics[2]:", topics[2].String())
	//	obj.From = strings.TrimPrefix(topics[1].String(), "000000000000000000000000")
	//	obj.To = strings.TrimPrefix(topics[2].String(), "000000000000000000000000")
	//	obj.From = HashAddressToBaseAddress(obj.From, isTestnet)
	//	obj.To = HashAddressToBaseAddress(obj.To, isTestnet)
	//
	//	//转化为10进制
	//	//log.Debug("TokenReceipt TxHash", obj.TxHash)
	//	//log.Debug("TokenReceipt amount", logInfo.Get("data").String())
	//	value := new(big.Int)
	//	value, _ = value.SetString(data, 16)
	//	obj.Amount = decimal.NewFromBigInt(value, 0).String()
	//}

	return &obj
}

//getBalanceByExplorer 获取地址余额
func (wm *WalletManager) getBalanceByExplorer(address string) (*openwallet.Balance, error) {

	path := fmt.Sprintf("address/%s", address)

	result, err := wm.ExplorerClient.Call(path, nil, "GET")
	if err != nil {
		return nil, err
	}

	return wm.newBalanceByExplorer(result), nil
}

func (wm *WalletManager) newBalanceByExplorer(json *gjson.Result) *openwallet.Balance {

	obj := openwallet.Balance{}
	//解析json
	//obj.Address = gjson.Get(json.Raw, "addrStr").String()
	u, _ := decimal.NewFromString(gjson.Get(json.Raw, "unconfirmed").String())
	b, _ := decimal.NewFromString(gjson.Get(json.Raw, "balance").String())
	obj.Balance = b.Shift(-wm.Decimal()).String()
	obj.UnconfirmBalance = u.Shift(-wm.Decimal()).String()
	obj.ConfirmBalance = b.Sub(u).String()

	return &obj
}

//getBalanceByExplorer 获取地址余额
func (wm *WalletManager) getBalanceCalUnspentByExplorer(address ...string) ([]*openwallet.Balance, error) {

	utxos, err := wm.listUnspentByExplorer(address...)
	if err != nil {
		return nil, err
	}

	addrBalanceMap := wm.calculateUnspentByExplorer(utxos)
	addrBalanceArr := make([]*openwallet.Balance, 0)
	for _, a := range address {

		var obj *openwallet.Balance
		if b, exist := addrBalanceMap[a]; exist {
			obj = b
		} else {
			obj = &openwallet.Balance{
				Symbol:           wm.Symbol(),
				Address:          a,
				Balance:          "0",
				UnconfirmBalance: "0",
				ConfirmBalance:   "0",
			}
		}

		addrBalanceArr = append(addrBalanceArr, obj)
	}

	return addrBalanceArr, nil
}

//calculateUnspentByExplorer 通过未花计算余额
func (wm *WalletManager) calculateUnspentByExplorer(utxos []*Unspent) map[string]*openwallet.Balance {

	addrBalanceMap := make(map[string]*openwallet.Balance)

	for _, utxo := range utxos {

		obj, exist := addrBalanceMap[utxo.Address]
		if !exist {
			obj = &openwallet.Balance{}
		}

		tu, _ := decimal.NewFromString(obj.UnconfirmBalance)
		tb, _ := decimal.NewFromString(obj.ConfirmBalance)

		if utxo.Spendable {
			if utxo.Confirmations > 0 {
				b, _ := decimal.NewFromString(utxo.Amount)
				tb = tb.Add(b)
			} else {
				u, _ := decimal.NewFromString(utxo.Amount)
				tu = tu.Add(u)
			}
		}

		obj.Symbol = wm.Symbol()
		obj.Address = utxo.Address
		obj.ConfirmBalance = tb.String()
		obj.UnconfirmBalance = tu.String()
		obj.Balance = tb.Add(tu).String()

		addrBalanceMap[utxo.Address] = obj
	}

	return addrBalanceMap

}

//getMultiAddrTransactionsByExplorer 获取多个地址的交易单数组
func (wm *WalletManager) getMultiAddrTransactionsByExplorer(offset, limit int, address ...string) ([]*Transaction, error) {

	var (
		trxs = make([]*Transaction, 0)
	)

	addrs := strings.Join(address, ",")

	request := req.Param{
		"addrs": addrs,
		"from":  offset,
		"to":    offset + limit,
	}

	path := fmt.Sprintf("addrs/txs")

	result, err := wm.ExplorerClient.Call(path, request, "POST")
	if err != nil {
		return nil, err
	}

	if items := result.Get("items"); items.IsArray() {
		for _, obj := range items.Array() {
			tx := wm.newTxByExplorer(&obj, wm.Config.isTestNet)
			trxs = append(trxs, tx)
		}
	}

	return trxs, nil
}

//estimateFeeRateByExplorer 通过浏览器获取费率
func (wm *WalletManager) estimateFeeRateByExplorer() (decimal.Decimal, error) {

	path := "info"

	result, err := wm.ExplorerClient.Call(path, nil, "GET")
	if err != nil {
		return decimal.New(0, 0), err
	}

	feeRate, _ := decimal.NewFromString(result.Get("feeRate").String())

	return feeRate, nil
}

//getTxOutByExplorer 获取交易单输出信息，用于追溯交易单输入源头
func (wm *WalletManager) getTxOutByExplorer(txid string, vout uint64) (*Vout, error) {

	tx, err := wm.getTransactionByExplorer(txid)
	if err != nil {
		return nil, err
	}

	for i, out := range tx.Vouts {
		if uint64(i) == vout {
			return out, nil
		}
	}

	return nil, fmt.Errorf("can not find ouput")

}

//sendRawTransactionByExplorer 广播交易
func (wm *WalletManager) sendRawTransactionByExplorer(txHex string) (string, error) {

	request := req.Param{
		"rawtx": txHex,
	}

	path := fmt.Sprintf("tx/send")

	result, err := wm.ExplorerClient.Call(path, request, "POST")
	if err != nil {
		return "", err
	}
	status := result.Get("status").Int()
	id := result.Get("id").String()
	message := result.Get("message").String()
	if status == 1 {
		return "", fmt.Errorf(message)
	}

	return id, nil

}

//getAddressTokenBalanceByExplorer 通过合约地址查询用户地址的余额
func (wm *WalletManager) getAddressTokenBalanceByExplorer(token openwallet.SmartContract, address string) (decimal.Decimal, error) {

	trimContractAddr := strings.TrimPrefix(token.Address, "0x")

	//tokenAddressBase := HashAddressToBaseAddress(trimContractAddr, wm.Config.isTestNet)

	path := fmt.Sprintf("address/%s", address)

	result, err := wm.ExplorerClient.Call(path, nil, "GET")
	if err != nil {
		return decimal.New(0, 0), err
	}

	if qrc20Balances := result.Get("qrc20Balances"); qrc20Balances.IsArray() {
		for _, qrc20 := range qrc20Balances.Array() {
			contractAddr := qrc20.Get("addressHex").String()
			if contractAddr == trimContractAddr {
				balanceStr := qrc20.Get("balance").String()
				balance, _ := decimal.NewFromString(balanceStr)
				balance = balance.Shift(int32(-token.Decimals))
				return balance, nil
			}
		}
	}

	return decimal.New(0, 0), nil

}
