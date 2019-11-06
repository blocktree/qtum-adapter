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
	"testing"
	"github.com/pborman/uuid"
)

func TestGetBTCBlockHeight(t *testing.T) {
	height, err := tw.GetBlockHeight()
	if err != nil {
		t.Errorf("GetBlockHeight failed unexpected error: %v\n", err)
		return
	}
	t.Logf("GetBlockHeight height = %d \n", height)
}

func TestBTCBlockScanner_GetCurrentBlockHeight(t *testing.T) {
	bs := NewBTCBlockScanner(tw)
	header, _ := bs.GetCurrentBlockHeader()
	t.Logf("GetCurrentBlockHeight height = %d \n", header.Height)
	t.Logf("GetCurrentBlockHeight hash = %v \n", header.Hash)
}

func TestGetBlockHeight(t *testing.T) {
	height, _ := tw.GetBlockHeight()
	t.Logf("GetBlockHeight height = %d \n", height)
}

func TestGetLocalNewBlock(t *testing.T) {
	height, hash := tw.GetLocalNewBlock()
	t.Logf("GetLocalBlockHeight height = %d \n", height)
	t.Logf("GetLocalBlockHeight hash = %v \n", hash)
}

func TestSaveLocalBlockHeight(t *testing.T) {
	bs := NewBTCBlockScanner(tw)
	header, _ := bs.GetCurrentBlockHeader()
	t.Logf("SaveLocalBlockHeight height = %d \n", header.Height)
	t.Logf("GetLocalBlockHeight hash = %v \n", header.Hash)
	tw.SaveLocalNewBlock(header.Height, header.Hash)
}

func TestGetBlockHash(t *testing.T) {
	//height := GetLocalBlockHeight()
	hash, err := tw.GetBlockHash(471507)
	if err != nil {
		t.Errorf("GetBlockHash failed unexpected error: %v\n", err)
		return
	}
	t.Logf("GetBlockHash hash = %s \n", hash)
}

func TestGetBlock(t *testing.T) {
	raw, err := tw.GetBlock("d91ee1ab39e55fd0dfdca242d44931ea4aa1cdc354b70b9e4531dae435da80dc")
	if err != nil {
		t.Errorf("GetBlock failed unexpected error: %v\n", err)
		return
	}
	t.Logf("GetBlock = %v \n", raw)
}

func TestGetTransaction(t *testing.T) {
	raw, err := tw.GetTransaction("1385cd40eaf4565a2de03af63ea5dc31672515fae33e12c7defcfc6477254328")
	if err != nil {
		t.Errorf("GetTransaction failed unexpected error: %v\n", err)
		return
	}

	t.Logf("BlockHash = %v \n", raw.BlockHash)
	t.Logf("BlockHeight = %v \n", raw.BlockHeight)
	t.Logf("Blocktime = %v \n", raw.Blocktime)
	t.Logf("Fees = %v \n", raw.Fees)

	t.Logf("========= vins ========= \n")

	for i, vin := range raw.Vins {
		t.Logf("TxID[%d] = %v \n", i, vin.TxID)
		t.Logf("Vout[%d] = %v \n", i, vin.Vout)
		t.Logf("Addr[%d] = %v \n", i, vin.Addr)
		t.Logf("Value[%d] = %v \n", i, vin.Value)
	}

	t.Logf("========= vouts ========= \n")

	for i, out := range raw.Vouts {
		t.Logf("ScriptPubKey[%d] = %v \n", i, out.ScriptPubKey)
		t.Logf("Addr[%d] = %v \n", i, out.Addr)
		t.Logf("Value[%d] = %v \n", i, out.Value)
		t.Logf("Type[%d] = %v \n", i, out.Type)
	}

	t.Logf("========= TokenReceipts ========= \n")

	for i, receipt := range raw.TokenReceipts {
		t.Logf("From[%d] = %v \n", i, receipt.From)
		t.Logf("To[%d] = %v \n", i, receipt.To)
		t.Logf("Value[%d] = %v \n", i, receipt.Amount)
		t.Logf("Contract[%d] = %v \n", i, receipt.ContractAddress)
	}

}


func TestGetTxOut(t *testing.T) {
	raw, err := tw.GetTxOut("abaa7238ce271bb9371a010c49bf86506e82f757dc9436932ab9975bccc4e30c", 0)
	if err != nil {
		t.Errorf("GetTxOut failed unexpected error: %v\n", err)
		return
	}
	t.Logf("GetTxOut = %v \n", raw)
}

func TestGetTxIDsInMemPool(t *testing.T) {
	txids, err := tw.GetTxIDsInMemPool()
	if err != nil {
		t.Errorf("GetTxIDsInMemPool failed unexpected error: %v\n", err)
		return
	}
	t.Logf("GetTxIDsInMemPool = %v \n", txids)
}

func TestBTCBlockScanner_scanning(t *testing.T) {

	//accountID := "WG8QXeEW7CVmRRbvw7Yb2f9wQf9ufR32M3"
	//address := "QYdBJS91qbE4jzjUttKn5R2DRaZM4dxz4D"

	//wallet, err := tw.GetWalletInfo(accountID)
	//if err != nil {
	//	t.Errorf("BTCBlockScanner_scanning failed unexpected error: %v\n", err)
	//	return
	//}

	bs := NewBTCBlockScanner(tw)

	//bs.DropRechargeRecords(accountID)

	bs.SetRescanBlockHeight(236523)
	//tw.SaveLocalNewBlock(1355030, "00000000000000125b86abb80b1f94af13a5d9b07340076092eda92dade27686")

	//bs.AddAddress(address, accountID)

	bs.ScanBlockTask()
}

func TestBTCBlockScanner_Run(t *testing.T) {

	var (
		endRunning = make(chan bool, 1)
	)

	//accountID := "WJwzaG2G4LoyuEb7NWAYiDa6DbtARtbUGv"
	//address := "QYdBJS91qbE4jzjUttKn5R2DRaZM4dxz4D"

	//wallet, err := tw.GetWalletInfo(accountID)
	//if err != nil {
	//	t.Errorf("BTCBlockScanner_Run failed unexpected error: %v\n", err)
	//	return
	//}

	bs := NewBTCBlockScanner(tw)

	//bs.DropRechargeRecords(accountID)

	bs.SetRescanBlockHeight(236523)

	//bs.AddAddress(address, accountID)

	bs.Run()

	<- endRunning

}

func TestBTCBlockScanner_ScanBlock(t *testing.T) {

	//accountID := "WJwzaG2G4LoyuEb7NWAYiDa6DbtARtbUGv"
	//address := "QhXS93hPpUcjoDxo192bmrbDubhH5UoQDp"

	bs := tw.blockscanner
	//bs.AddAddress(address, accountID)
	bs.ScanBlock(249878)
}

func TestBTCBlockScanner_ExtractTransaction(t *testing.T) {

	//accountID := "WJwzaG2G4LoyuEb7NWAYiDa6DbtARtbUGv"
	//address := "Qhuqn4r1Xj8tjn3s1gAqh1aBZKam99h6iF"
	//
	//bs := tw.blockscanner
	//bs.AddAddress(address, accountID)
	//bs.ExtractTransaction(
	//	231425,
	//	"7b12f59d92b3f4d4b4793df691942ab9ab2621187a751b875df983b4a22f73a5",
	//	"9beb4c5ceacc5dfb90f299bfc737e1a00b0faecd9eb62c4258f9a94a93417a2a",
	//	bs.GetSourceKeyByAddress)

}

func TestWallet_GetRecharges(t *testing.T) {
	accountID := "WG8QXeEW7CVmRRbvw7Yb2f9wQf9ufR32M3"
	wallet, err := tw.GetWalletInfo(accountID)
	if err != nil {
		t.Errorf("GetWalletInfo failed unexpected error: %v\n", err)
		return
	}

	recharges, err := wallet.GetRecharges(false)
	if err != nil {
		t.Errorf("GetRecharges failed unexpected error: %v\n", err)
		return
	}

	t.Logf("recharges.count = %v", len(recharges))
	//for _, r := range recharges {
	//	t.Logf("rechanges.count = %v", len(r))
	//}
}

//func TestBTCBlockScanner_DropRechargeRecords(t *testing.T) {
//	accountID := "W4ruoAyS5HdBMrEeeHQTBxo4XtaAixheXQ"
//	bs := NewBTCBlockScanner(tw)
//	bs.DropRechargeRecords(accountID)
//}

func TestGetUnscanRecords(t *testing.T) {
	list, err := tw.GetUnscanRecords()
	if err != nil {
		t.Errorf("GetUnscanRecords failed unexpected error: %v\n", err)
		return
	}

	for _, r := range list {
		t.Logf("GetUnscanRecords unscan: %v", r)
	}
}

func TestBTCBlockScanner_RescanFailedRecord(t *testing.T) {
	bs := NewBTCBlockScanner(tw)
	bs.RescanFailedRecord()
}

func TestFullAddress (t *testing.T) {

	dic := make(map[string]string)
	for i := 0;i<20000000;i++ {
		dic[uuid.NewUUID().String()] = uuid.NewUUID().String()
	}
}

func TestBTCBlockScanner_GetBalanceByAddress(t *testing.T) {

	addrs := []string{
		//"qVT4jAoQDJ6E4FbjW1HPcwgXuF2ZdM2CAP",
		//"qQLYQn7vCAU8irPEeqjZ3rhFGLnS5vxVy8",
		//"qMXS1YFtA5qr2UfhcDMthTCK6hWhJnzC47",
		//"qJq5GbHeaaNbi6Bs5QCbuCZsZRXVWPoG1k",
		"QVTQ8QaKXcEzsGX74JePUkge5K4r41Ey3v",
		"Qf2tU1GExXe8smNyoMgis9u1CLnQ2q1mam",
	}

	//contract := openwallet.SmartContract{
	//	Address: "482be94ca327f1dd1d9857a5a212df091f44980f",
	//}
	//addrs := []string{
	//	"qUaHAjfRLknMBuSsA5kBfkn9xLMDFc2FdV",
	//	"qJ2HTPYoMF1DPBhgURjRqemun5WimD57Hy",
	//}

	balanceList, err := tw.blockscanner.GetBalanceByAddress(addrs...)
	if err != nil {
		t.Errorf("get token balance by address failed, err=%v", err)
		return
	}

	//输出json格式
	//objStr, _ := json.MarshalIndent(balanceList, "", " ")
	//t.Logf("balance list:%v", string(objStr))

	for i:=0; i<len(balanceList); i++ {
		t.Logf("%s: %s\n",addrs[i], balanceList[i].Balance)
	}
}