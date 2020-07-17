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
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/blocktree/openwallet/v2/common"
	"github.com/blocktree/openwallet/v2/openwallet"
	"github.com/graarh/golang-socketio"
	"github.com/graarh/golang-socketio/transport"
	"github.com/shopspring/decimal"
)

const (
	//blockchainBucket = "blockchain" //区块链数据集合
	//periodOfTask      = 5 * time.Second //定时任务执行隔间
	maxExtractingSize = 15 //并发的扫描线程数
)

//BTCBlockScanner bitcoin的区块链扫描器
type BTCBlockScanner struct {
	*openwallet.BlockScannerBase

	CurrentBlockHeight   uint64             //当前区块高度
	extractingCH         chan struct{}      //扫描工作令牌
	wm                   *WalletManager     //钱包管理者
	IsScanMemPool        bool               //是否扫描交易池
	RescanLastBlockCount uint64             //重扫上N个区块数量
	socketIO             *gosocketio.Client //socketIO客户端
	stopSocketIO         chan struct{}
}

//ExtractResult 扫描完成的提取结果
type ExtractResult struct {
	extractData         map[string]*openwallet.TxExtractData //主链交易
	extractContractData map[string]*openwallet.TxExtractData //代币交易
	TxID                string
	BlockHeight         uint64
	Success             bool
}

//SaveResult 保存结果
type SaveResult struct {
	TxID        string
	BlockHeight uint64
	Success     bool
}

//NewBTCBlockScanner 创建区块链扫描器
func NewBTCBlockScanner(wm *WalletManager) *BTCBlockScanner {
	bs := BTCBlockScanner{
		BlockScannerBase: openwallet.NewBlockScannerBase(),
	}

	bs.extractingCH = make(chan struct{}, maxExtractingSize)
	bs.wm = wm
	bs.IsScanMemPool = false
	bs.RescanLastBlockCount = 0
	bs.stopSocketIO = make(chan struct{})

	//设置扫描任务
	bs.SetTask(bs.ScanBlockTask)

	return &bs
}

//SetRescanBlockHeight 重置区块链扫描高度
func (bs *BTCBlockScanner) SetRescanBlockHeight(height uint64) error {
	height = height - 1
	if height < 0 {
		return errors.New("block height to rescan must greater than 0.")
	}

	hash, err := bs.wm.GetBlockHash(height)
	if err != nil {
		return err
	}

	bs.SaveLocalNewBlock(height, hash)

	return nil
}

//ScanBlockTask 扫描任务
func (bs *BTCBlockScanner) ScanBlockTask() {

	//获取本地区块高度
	blockHeader, err := bs.GetScannedBlockHeader()
	if err != nil {
		bs.wm.Log.Std.Info("block scanner can not get new block height; unexpected error: %v", err)
		return
	}

	currentHeight := blockHeader.Height
	currentHash := blockHeader.Hash

	for {

		if !bs.Scanning {
			//区块扫描器已暂停，马上结束本次任务
			return
		}

		//获取最大高度
		maxHeight, err := bs.wm.GetBlockHeight()
		if err != nil {
			//下一个高度找不到会报异常
			bs.wm.Log.Std.Info("block scanner can not get rpc-server block height; unexpected error: %v", err)
			break
		}

		//是否已到最新高度
		if currentHeight >= maxHeight {
			bs.wm.Log.Std.Info("block scanner has scanned full chain data. Current height: %d", maxHeight)
			break
		}

		//继续扫描下一个区块
		currentHeight = currentHeight + 1

		bs.wm.Log.Std.Info("block scanner scanning height: %d ...", currentHeight)

		hash, err := bs.wm.GetBlockHash(currentHeight)
		if err != nil {
			//下一个高度找不到会报异常
			bs.wm.Log.Std.Info("block scanner can not get new block hash; unexpected error: %v", err)
			break
		}

		block, err := bs.wm.GetBlock(hash)
		if err != nil {
			bs.wm.Log.Std.Info("block scanner can not get new block data; unexpected error: %v", err)

			//记录未扫区块
			unscanRecord := openwallet.NewUnscanRecord(currentHeight, "", err.Error(), bs.wm.Symbol())
			bs.SaveUnscanRecord(unscanRecord)
			bs.wm.Log.Std.Info("block height: %d extract failed.", currentHeight)
			continue
		}

		isFork := false

		//判断hash是否上一区块的hash
		if currentHash != block.Previousblockhash {

			bs.wm.Log.Std.Info("block has been fork on height: %d.", currentHeight)
			bs.wm.Log.Std.Info("block height: %d local hash = %s ", currentHeight-1, currentHash)
			bs.wm.Log.Std.Info("block height: %d mainnet hash = %s ", currentHeight-1, block.Previousblockhash)

			bs.wm.Log.Std.Info("delete recharge records on block height: %d.", currentHeight-1)

			//删除上一区块链的所有充值记录
			//bs.DeleteRechargesByHeight(currentHeight - 1)
			//删除上一区块链的未扫记录

			//查询本地分叉的区块
			forkBlock, _ := bs.GetLocalBlock(currentHeight - 1)

			bs.DeleteUnscanRecord(currentHeight - 1)
			currentHeight = currentHeight - 2 //倒退2个区块重新扫描
			if currentHeight <= 0 {
				currentHeight = 1
			}

			localBlock, err := bs.GetLocalBlock(currentHeight)
			if err != nil {
				bs.wm.Log.Std.Error("block scanner can not get local block; unexpected error: %v", err)

				//查找core钱包的RPC
				bs.wm.Log.Info("block scanner prev block height:", currentHeight)

				prevHash, err := bs.wm.GetBlockHash(currentHeight)
				if err != nil {
					bs.wm.Log.Std.Error("block scanner can not get prev block; unexpected error: %v", err)
					break
				}

				localBlock, err = bs.wm.GetBlock(prevHash)
				if err != nil {
					bs.wm.Log.Std.Error("block scanner can not get prev block; unexpected error: %v", err)
					break
				}
			}

			//重置当前区块的hash
			currentHash = localBlock.Hash

			bs.wm.Log.Std.Info("rescan block on height: %d, hash: %s .", currentHeight, currentHash)

			//重新记录一个新扫描起点
			bs.SaveLocalNewBlock(localBlock.Height, localBlock.Hash)

			isFork = true

			if forkBlock != nil {

				//通知分叉区块给观测者，异步处理
				bs.newBlockNotify(forkBlock, isFork)
			}

		} else {

			err = bs.BatchExtractTransaction(block.Height, block.Hash, block.tx)
			if err != nil {
				bs.wm.Log.Std.Info("block scanner can not extractRechargeRecords; unexpected error: %v", err)
			}

			//重置当前区块的hash
			currentHash = hash

			//保存本地新高度
			bs.SaveLocalNewBlock(currentHeight, currentHash)
			bs.SaveLocalBlock(block)

			isFork = false

			//通知新区块给观测者，异步处理
			bs.newBlockNotify(block, isFork)
		}

	}

	//重扫前N个块，为保证记录找到
	for i := currentHeight - bs.RescanLastBlockCount; i < currentHeight; i++ {
		bs.scanBlock(i)
	}

	if bs.IsScanMemPool {
		//扫描交易内存池
		bs.ScanTxMemPool()
	}

	//重扫失败区块
	bs.RescanFailedRecord()

}

//ScanBlock 扫描指定高度区块
func (bs *BTCBlockScanner) ScanBlock(height uint64) error {

	block, err := bs.scanBlock(height)
	if err != nil {
		return err
	}

	//通知新区块给观测者，异步处理
	bs.newBlockNotify(block, false)

	return nil
}

func (bs *BTCBlockScanner) scanBlock(height uint64) (*Block, error) {

	hash, err := bs.wm.GetBlockHash(height)
	if err != nil {
		//下一个高度找不到会报异常
		bs.wm.Log.Std.Info("block scanner can not get new block hash; unexpected error: %v", err)
		return nil, err
	}

	block, err := bs.wm.GetBlock(hash)
	if err != nil {
		bs.wm.Log.Std.Info("block scanner can not get new block data; unexpected error: %v", err)

		//记录未扫区块
		unscanRecord := openwallet.NewUnscanRecord(height, "", err.Error(), bs.wm.Symbol())
		bs.SaveUnscanRecord(unscanRecord)
		bs.wm.Log.Std.Info("block height: %d extract failed.", height)
		return nil, err
	}

	bs.wm.Log.Std.Info("block scanner scanning height: %d ...", block.Height)

	err = bs.BatchExtractTransaction(block.Height, block.Hash, block.tx)
	if err != nil {
		bs.wm.Log.Std.Info("block scanner can not extractRechargeRecords; unexpected error: %v", err)
	}

	//保存区块
	//bs.wm.SaveLocalBlock(block)

	return block, nil
}

//ScanTxMemPool 扫描交易内存池
func (bs *BTCBlockScanner) ScanTxMemPool() {

	bs.wm.Log.Std.Info("block scanner scanning mempool ...")

	//提取未确认的交易单
	txIDsInMemPool, err := bs.wm.GetTxIDsInMemPool()
	if err != nil {
		bs.wm.Log.Std.Info("block scanner can not get mempool data; unexpected error: %v", err)
		return
	}

	if txIDsInMemPool == nil || len(txIDsInMemPool) == 0 {
		return
	}

	err = bs.BatchExtractTransaction(0, "", txIDsInMemPool)
	if err != nil {
		bs.wm.Log.Std.Info("block scanner can not extractRechargeRecords; unexpected error: %v", err)
	}

}

//rescanFailedRecord 重扫失败记录
func (bs *BTCBlockScanner) RescanFailedRecord() {

	var (
		blockMap = make(map[uint64][]string)
	)

	list, err := bs.GetUnscanRecords()
	if err != nil {
		bs.wm.Log.Std.Info("block scanner can not get rescan data; unexpected error: %v", err)
	}

	//组合成批处理
	for _, r := range list {

		if _, exist := blockMap[r.BlockHeight]; !exist {
			blockMap[r.BlockHeight] = make([]string, 0)
		}

		if len(r.TxID) > 0 {
			arr := blockMap[r.BlockHeight]
			arr = append(arr, r.TxID)

			blockMap[r.BlockHeight] = arr
		}
	}

	for height, txs := range blockMap {

		if height == 0 {
			continue
		}

		var hash string

		bs.wm.Log.Std.Info("block scanner rescanning height: %d ...", height)

		if len(txs) == 0 {

			hash, err := bs.wm.GetBlockHash(height)
			if err != nil {
				//下一个高度找不到会报异常
				bs.wm.Log.Std.Info("block scanner can not get new block hash; unexpected error: %v", err)
				continue
			}

			block, err := bs.wm.GetBlock(hash)
			if err != nil {
				bs.wm.Log.Std.Info("block scanner can not get new block data; unexpected error: %v", err)
				continue
			}

			txs = block.tx
		}

		err = bs.BatchExtractTransaction(height, hash, txs)
		if err != nil {
			bs.wm.Log.Std.Info("block scanner can not extractRechargeRecords; unexpected error: %v", err)
			continue
		}

		//删除未扫记录
		bs.DeleteUnscanRecord(height)
	}

	//删除未没有找到交易记录的重扫记录
	bs.DeleteUnscanRecordNotFindTX()
}

//newBlockNotify 获得新区块后，通知给观测者
func (bs *BTCBlockScanner) newBlockNotify(block *Block, isFork bool) {
	header := block.BlockHeader(bs.wm.Symbol())
	header.Fork = isFork
	bs.NewBlockNotify(header)
}

//BatchExtractTransaction 批量提取交易单
//bitcoin 1M的区块链可以容纳3000笔交易，批量多线程处理，速度更快
func (bs *BTCBlockScanner) BatchExtractTransaction(blockHeight uint64, blockHash string, txs []string) error {

	var (
		quit       = make(chan struct{})
		done       = 0 //完成标记
		failed     = 0
		shouldDone = len(txs) //需要完成的总数
	)

	if len(txs) == 0 {
		return errors.New("BatchExtractTransaction block is nil.")
	}


	//生产通道
	producer := make(chan ExtractResult)
	defer close(producer)

	//消费通道
	worker := make(chan ExtractResult)
	defer close(worker)

	//保存工作
	saveWork := func(height uint64, result chan ExtractResult) {
		//回收创建的地址
		for gets := range result {

			if gets.Success {

				notifyErr := bs.newExtractDataNotify(height, gets.extractData)
				//saveErr := bs.SaveRechargeToWalletDB(height, gets.Recharges)
				if notifyErr != nil {
					failed++ //标记保存失败数
					bs.wm.Log.Std.Info("newExtractDataNotify unexpected error: %v", notifyErr)
				}

				notifyErr = nil
				notifyErr = bs.newExtractDataNotify(height, gets.extractContractData)
				if notifyErr != nil {
					failed++ //标记保存失败数
					bs.wm.Log.Std.Info("newExtractDataNotify unexpected error: %v", notifyErr)
				}

			} else {
				//记录未扫区块
				unscanRecord := openwallet.NewUnscanRecord(height, "", "", bs.wm.Symbol())
				bs.SaveUnscanRecord(unscanRecord)
				//bs.wm.Log.Std.Info("block height: %d extract failed.", height)
				failed++ //标记保存失败数
			}
			//累计完成的线程数
			done++
			if done == shouldDone {
				//bs.wm.Log.Std.Info("done = %d, shouldDone = %d ", done, len(txs))
				close(quit) //关闭通道，等于给通道传入nil
			}
		}
	}

	//提取工作
	extractWork := func(eblockHeight uint64, eBlockHash string, mTxs []string, eProducer chan ExtractResult) {
		for i, txid := range mTxs {
			bs.extractingCH <- struct{}{}
			//第一笔交易是coinbase，第二笔交易是pos收益
			isCoinstake := false
			if i == 1 {
				isCoinstake = true
			}
			go func(mBlockHeight uint64, mTxid string, isCoinstake bool, end chan struct{}, mProducer chan<- ExtractResult) {

				//导出提出的交易
				mProducer <- bs.ExtractTransaction(mBlockHeight, eBlockHash, mTxid, isCoinstake, bs.ScanTargetFuncV2)
				//释放
				<-end

			}(eblockHeight, txid, isCoinstake, bs.extractingCH, eProducer)
		}
	}

	/*	开启导出的线程	*/

	//独立线程运行消费
	go saveWork(blockHeight, worker)

	//独立线程运行生产
	go extractWork(blockHeight, blockHash, txs, producer)

	//以下使用生产消费模式
	bs.extractRuntime(producer, worker, quit)

	if failed > 0 {
		return fmt.Errorf("block scanner saveWork failed")
	} else {
		return nil
	}

	//return nil
}

//extractRuntime 提取运行时
func (bs *BTCBlockScanner) extractRuntime(producer chan ExtractResult, worker chan ExtractResult, quit chan struct{}) {

	var (
		values = make([]ExtractResult, 0)
	)

	for {

		var activeWorker chan<- ExtractResult
		var activeValue ExtractResult

		//当数据队列有数据时，释放顶部，传输给消费者
		if len(values) > 0 {
			activeWorker = worker
			activeValue = values[0]

		}

		select {

		//生成者不断生成数据，插入到数据队列尾部
		case pa := <-producer:
			values = append(values, pa)
		case <-quit:
			//退出
			//bs.wm.Log.Std.Info("block scanner have been scanned!")
			return
		case activeWorker <- activeValue:
			//wm.Log.Std.Info("Get %d", len(activeValue))
			values = values[1:]
		}
	}

	return

}

//ExtractTransaction 提取交易单
func (bs *BTCBlockScanner) ExtractTransaction(blockHeight uint64, blockHash string, txid string, isCoinstake bool, scanAddressFunc openwallet.BlockScanTargetFuncV2) ExtractResult {

	var (
		result = ExtractResult{
			BlockHeight:         blockHeight,
			TxID:                txid,
			extractData:         make(map[string]*openwallet.TxExtractData),
			extractContractData: make(map[string]*openwallet.TxExtractData),
		}
	)

	//bs.wm.Log.Std.Debug("block scanner scanning tx: %s ...", txid)
	trx, err := bs.wm.GetTransaction(txid)

	if err != nil {
		bs.wm.Log.Std.Info("block scanner can not extract transaction data; unexpected error: %v", err)
		result.Success = false
		return result
	}

	//优先使用传入的高度
	if blockHeight > 0 && trx.BlockHeight == 0 {
		trx.BlockHeight = blockHeight
		trx.BlockHash = blockHash
	}
	//提取主币交易单
	bs.extractTransaction(trx, &result, scanAddressFunc)
	//提取代币交易单
	bs.extractTokenTransfer(trx, &result, scanAddressFunc)
	return result

}

//ExtractTransactionData 提取交易单
func (bs *BTCBlockScanner) extractTransaction(trx *Transaction, result *ExtractResult, scanAddressFunc openwallet.BlockScanTargetFuncV2) {

	var (
		success = true
	)

	if trx == nil {
		//记录哪个区块哪个交易单没有完成扫描
		success = false
	} else {

		vin := trx.Vins
		blocktime := trx.Blocktime

		//检查交易单输入信息是否完整，不完整查上一笔交易单的输出填充数据
		for _, input := range vin {

			if trx.IsCoinBase {
				//coinbase skip
				success = true
				break
			}

			//如果input中没有地址，需要查上一笔交易的output提取
			if len(input.Addr) == 0 {

				intxid := input.TxID
				vout := input.Vout

				preTx, err := bs.wm.GetTransaction(intxid)
				if err != nil {
					success = false
					break
				} else {
					preVouts := preTx.Vouts
					if len(preVouts) > int(vout) {
						preOut := preVouts[vout]
						input.Addr = preOut.Addr
						input.Value = preOut.Value
						//vinout = append(vinout, output[vout])
						success = true

						//bs.wm.Log.Debug("GetTxOut:", output[vout])

					}
				}

			}

		}

		if success {

			txType := uint64(0)
			txAction := ""

			if trx.Isqrc20Transfer {
				txType = 1
				txAction = "transfer"
			}

			if trx.IsCoinstake {
				txType = 100
				txAction = "coinstake"
			}

			//提取出账部分记录
			from, totalSpent := bs.extractTxInput(trx, trx.IsCoinstake, result, scanAddressFunc)
			//bs.wm.Log.Debug("from:", from, "totalSpent:", totalSpent)

			//提取入账部分记录
			to, totalReceived := bs.extractTxOutput(trx, trx.IsCoinstake, result, scanAddressFunc)
			//bs.wm.Log.Debug("to:", to, "totalReceived:", totalReceived)

			for _, extractData := range result.extractData {
				tx := &openwallet.Transaction{
					From: from,
					To:   to,
					Fees: totalSpent.Sub(totalReceived).StringFixed(8),
					Coin: openwallet.Coin{
						Symbol:     bs.wm.Symbol(),
						IsContract: false,
					},
					BlockHash:   trx.BlockHash,
					BlockHeight: trx.BlockHeight,
					TxID:        trx.TxID,
					Decimal:     8,
					ConfirmTime: blocktime,
					Status:      openwallet.TxStatusSuccess,
					TxType:      txType,
					TxAction:    txAction,
				}
				wxID := openwallet.GenTransactionWxID(tx)
				tx.WxID = wxID
				extractData.Transaction = tx

				//bs.wm.Log.Debug("Transaction:", extractData.Transaction)
			}

		}

		success = true

	}
	result.Success = success

}

//ExtractTxInput 提取交易单输入部分
func (bs *BTCBlockScanner) extractTxInput(trx *Transaction, isCoinstake bool, result *ExtractResult, scanAddressFunc openwallet.BlockScanTargetFuncV2) ([]string, decimal.Decimal) {

	//vin := trx.Get("vin")

	var (
		from        = make([]string, 0)
		totalAmount = decimal.Zero
	)

	txType := uint64(0)

	if isCoinstake {
		txType = 100
	}

	createAt := time.Now().Unix()
	for i, output := range trx.Vins {

		//in := vin[i]

		txid := output.TxID
		vout := output.Vout
		//
		//output, err := bs.wm.GetTxOut(txid, vout)
		//if err != nil {
		//	return err
		//}

		amount := output.Value
		addr := output.Addr
		targetResult := scanAddressFunc(openwallet.ScanTargetParam{
			ScanTarget:     addr,
			Symbol:         bs.wm.Symbol(),
			ScanTargetType: openwallet.ScanTargetTypeAccountAddress})
		if targetResult.Exist {
			input := openwallet.TxInput{}
			input.SourceTxID = txid
			input.SourceIndex = vout
			input.TxID = result.TxID
			input.Address = addr
			//transaction.AccountID = a.AccountID
			input.Amount = amount
			input.Coin = openwallet.Coin{
				Symbol:     bs.wm.Symbol(),
				IsContract: false,
			}
			input.Index = output.N
			input.Sid = openwallet.GenTxInputSID(txid, bs.wm.Symbol(), "", uint64(i))
			//input.Sid = base64.StdEncoding.EncodeToString(crypto.SHA1([]byte(fmt.Sprintf("input_%s_%d_%s", result.TxID, i, addr))))
			input.CreateAt = createAt
			//在哪个区块高度时消费
			input.BlockHeight = trx.BlockHeight
			input.BlockHash = trx.BlockHash
			input.TxType = txType

			//transactions = append(transactions, &transaction)

			ed := result.extractData[targetResult.SourceKey]
			if ed == nil {
				ed = openwallet.NewBlockExtractData()
				result.extractData[targetResult.SourceKey] = ed
			}

			ed.TxInputs = append(ed.TxInputs, &input)

		}

		from = append(from, addr+":"+amount)
		dAmount, _ := decimal.NewFromString(amount)
		totalAmount = totalAmount.Add(dAmount)

	}
	return from, totalAmount
}

//ExtractTxInput 提取交易单输入部分
func (bs *BTCBlockScanner) extractTxOutput(trx *Transaction, isCoinstake bool, result *ExtractResult, scanAddressFunc openwallet.BlockScanTargetFuncV2) ([]string, decimal.Decimal) {

	var (
		to          = make([]string, 0)
		totalAmount = decimal.Zero
	)

	txType := uint64(0)

	if isCoinstake {
		txType = 100
	}

	confirmations := trx.Confirmations
	vout := trx.Vouts
	txid := trx.TxID
	//bs.wm.Log.Debug("vout:", vout.Array())
	createAt := time.Now().Unix()
	for _, output := range vout {

		amount := output.Value
		n := output.N
		addr := output.Addr
		targetResult := scanAddressFunc(openwallet.ScanTargetParam{
			ScanTarget:     addr,
			Symbol:         bs.wm.Symbol(),
			ScanTargetType: openwallet.ScanTargetTypeAccountAddress})
		if targetResult.Exist {

			//a := wallet.GetAddress(addr)
			//if a == nil {
			//	continue
			//}

			outPut := openwallet.TxOutPut{}
			outPut.TxID = txid
			outPut.Address = addr
			//transaction.AccountID = a.AccountID
			outPut.Amount = amount
			outPut.Coin = openwallet.Coin{
				Symbol:     bs.wm.Symbol(),
				IsContract: false,
			}
			outPut.Index = n
			outPut.Sid = openwallet.GenTxOutPutSID(txid, bs.wm.Symbol(), "", n)
			//outPut.Sid = base64.StdEncoding.EncodeToString(crypto.SHA1([]byte(fmt.Sprintf("output_%s_%d_%s", txid, n, addr))))

			//保存utxo到扩展字段
			outPut.SetExtParam("scriptPubKey", output.ScriptPubKey)
			outPut.CreateAt = createAt
			outPut.BlockHeight = trx.BlockHeight
			outPut.BlockHash = trx.BlockHash
			outPut.Confirm = int64(confirmations)
			outPut.TxType = txType

			//transactions = append(transactions, &transaction)

			ed := result.extractData[targetResult.SourceKey]
			if ed == nil {
				ed = openwallet.NewBlockExtractData()
				result.extractData[targetResult.SourceKey] = ed
			}

			ed.TxOutputs = append(ed.TxOutputs, &outPut)

		}

		to = append(to, addr+":"+amount)
		dAmount, _ := decimal.NewFromString(amount)
		totalAmount = totalAmount.Add(dAmount)

	}

	return to, totalAmount
}

//extractTokenTransfer 提取交易单中的代币交易
func (bs *BTCBlockScanner) extractTokenTransfer(trx *Transaction, result *ExtractResult, scanAddressFunc openwallet.BlockScanTargetFuncV2) {

	var (
		success = true
	)

	if trx == nil {
		//记录哪个区块哪个交易单没有完成扫描
		success = false
	} else {

		if trx.Isqrc20Transfer {
			createAt := time.Now().Unix()
			//QTUM交易单目前只允许包含一个代币交易
			for _, tokenReceipt := range trx.TokenReceipts {

				contractId := openwallet.GenContractID(bs.wm.Symbol(), tokenReceipt.ContractAddress)

				coin := openwallet.Coin{
					Symbol:     bs.wm.Symbol(),
					IsContract: true,
					ContractID: contractId,
					Contract: openwallet.SmartContract{
						ContractID: contractId,
						Address:    tokenReceipt.ContractAddress,
						Protocol:   "qrc20",
						Symbol:     bs.wm.Symbol(),
					},
				}

				targetResult := scanAddressFunc(openwallet.ScanTargetParam{
					ScanTarget:     tokenReceipt.From,
					Symbol:         bs.wm.Symbol(),
					ScanTargetType: openwallet.ScanTargetTypeAccountAddress})
				if targetResult.Exist {
					input := openwallet.TxInput{}
					input.TxID = trx.TxID
					input.Address = tokenReceipt.From
					//transaction.AccountID = a.AccountID
					input.Amount = tokenReceipt.Amount
					input.Coin = coin
					input.Index = 0
					input.Sid = openwallet.GenTxInputSID(tokenReceipt.TxHash, bs.wm.Symbol(), contractId, 0)
					//input.Sid = base64.StdEncoding.EncodeToString(crypto.SHA1([]byte(fmt.Sprintf("input_%s_%d_%s", result.TxID, i, addr))))
					input.CreateAt = createAt
					//在哪个区块高度时消费
					input.BlockHeight = tokenReceipt.BlockHeight
					input.BlockHash = tokenReceipt.BlockHash

					//transactions = append(transactions, &transaction)

					ed := result.extractContractData[targetResult.SourceKey]
					if ed == nil {
						ed = openwallet.NewBlockExtractData()
						result.extractContractData[targetResult.SourceKey] = ed
					}

					ed.TxInputs = append(ed.TxInputs, &input)

				}

				targetResult2 := scanAddressFunc(openwallet.ScanTargetParam{
					ScanTarget:     tokenReceipt.To,
					Symbol:         bs.wm.Symbol(),
					ScanTargetType: openwallet.ScanTargetTypeAccountAddress})
				if targetResult2.Exist {
					output := openwallet.TxOutPut{}
					output.TxID = trx.TxID
					output.Address = tokenReceipt.To
					//transaction.AccountID = a.AccountID
					output.Amount = tokenReceipt.Amount

					output.Coin = coin
					output.Index = 0
					output.Sid = openwallet.GenTxOutPutSID(tokenReceipt.TxHash, bs.wm.Symbol(), contractId, 0)
					//input.Sid = base64.StdEncoding.EncodeToString(crypto.SHA1([]byte(fmt.Sprintf("input_%s_%d_%s", result.TxID, i, addr))))
					output.CreateAt = createAt
					//在哪个区块高度时消费
					output.BlockHeight = tokenReceipt.BlockHeight
					output.BlockHash = tokenReceipt.BlockHash

					//transactions = append(transactions, &transaction)

					ed := result.extractContractData[targetResult2.SourceKey]
					if ed == nil {
						ed = openwallet.NewBlockExtractData()
						result.extractContractData[targetResult2.SourceKey] = ed
					}

					ed.TxOutputs = append(ed.TxOutputs, &output)
				}

				blocktime := trx.Blocktime

				for _, extractData := range result.extractContractData {
					tx := &openwallet.Transaction{
						From:        []string{tokenReceipt.From + ":" + tokenReceipt.Amount},
						To:          []string{tokenReceipt.To + ":" + tokenReceipt.Amount},
						Fees:        "0",
						Coin:        coin,
						BlockHash:   tokenReceipt.BlockHash,
						BlockHeight: tokenReceipt.BlockHeight,
						TxID:        tokenReceipt.TxHash,
						Decimal:     0,
						ConfirmTime: blocktime,
						Status:      openwallet.TxStatusSuccess,
						TxType:      0,
					}
					wxID := openwallet.GenTransactionWxID(tx)
					tx.WxID = wxID
					extractData.Transaction = tx

					//bs.wm.Log.Debug("Transaction:", extractData.Transaction)
				}
			}
		}

		success = true

	}

	result.Success = success

}

//newExtractDataNotify 发送通知
func (bs *BTCBlockScanner) newExtractDataNotify(height uint64, extractData map[string]*openwallet.TxExtractData) error {

	for o, _ := range bs.Observers {
		for key, data := range extractData {
			dataJson, _ := json.Marshal(data)
			bs.wm.Log.Infof("TxExtractData: %s", string(dataJson))
			err := o.BlockExtractDataNotify(key, data)
			if err != nil {
				bs.wm.Log.Error("BlockExtractDataNotify unexpected error:", err)
				//记录未扫区块
				unscanRecord := openwallet.NewUnscanRecord(height, "", "ExtractData Notify failed.", bs.wm.Symbol())
				err = bs.SaveUnscanRecord(unscanRecord)
				if err != nil {
					bs.wm.Log.Std.Error("block height: %d, save unscan record failed. unexpected error: %v", height, err.Error())
				}

			}
		}
	}

	return nil
}

//DeleteUnscanRecordNotFindTX 删除未没有找到交易记录的重扫记录
func (bs *BTCBlockScanner) DeleteUnscanRecordNotFindTX() error {

	//删除找不到交易单
	reason := "[-5]No information available about transaction"

	if bs.BlockchainDAI == nil {
		return fmt.Errorf("Blockchain DAI is not setup ")
	}

	list, err := bs.BlockchainDAI.GetUnscanRecords(bs.wm.Symbol())
	if err != nil {
		return err
	}

	for _, r := range list {
		if strings.HasPrefix(r.Reason, reason) {
			bs.BlockchainDAI.DeleteUnscanRecordByID(r.ID, bs.wm.Symbol())
		}
	}
	return nil
}

//GetScannedBlockHeader 获取当前已扫区块高度
func (bs *BTCBlockScanner) GetScannedBlockHeader() (*openwallet.BlockHeader, error) {

	var (
		blockHeight uint64 = 0
		hash        string
		err         error
	)

	blockHeight, hash, _ = bs.GetLocalNewBlock()

	//如果本地没有记录，查询接口的高度
	if blockHeight == 0 {
		blockHeight, err = bs.wm.GetBlockHeight()
		if err != nil {

			return nil, err
		}

		//就上一个区块链为当前区块
		blockHeight = blockHeight - 1

		hash, err = bs.wm.GetBlockHash(blockHeight)
		if err != nil {
			return nil, err
		}
	}

	return &openwallet.BlockHeader{Height: blockHeight, Hash: hash}, nil
}

//GetCurrentBlockHeader 获取当前区块高度
func (bs *BTCBlockScanner) GetCurrentBlockHeader() (*openwallet.BlockHeader, error) {

	var (
		blockHeight uint64 = 0
		hash        string
		err         error
	)

	blockHeight, err = bs.wm.GetBlockHeight()
	if err != nil {

		return nil, err
	}

	hash, err = bs.wm.GetBlockHash(blockHeight)
	if err != nil {
		return nil, err
	}

	return &openwallet.BlockHeader{Height: blockHeight, Hash: hash}, nil
}

//GetScannedBlockHeight 获取已扫区块高度
func (bs *BTCBlockScanner) GetScannedBlockHeight() uint64 {
	localHeight, _, _ := bs.GetLocalNewBlock()
	return localHeight
}

func (bs *BTCBlockScanner) ExtractTransactionData(txid string, scanTargetFunc openwallet.BlockScanTargetFunc) (map[string][]*openwallet.TxExtractData, error) {

	scanTargetFuncV2 := func(target openwallet.ScanTargetParam) openwallet.ScanTargetResult {
		sourceKey, ok := scanTargetFunc(openwallet.ScanTarget{
			Address:          target.ScanTarget,
			Symbol:           bs.wm.Symbol(),
			BalanceModelType: bs.wm.BalanceModelType(),
		})
		return openwallet.ScanTargetResult{
			SourceKey: sourceKey,
			Exist:     ok,
		}
	}

	result := bs.ExtractTransaction(0, "", txid, false, scanTargetFuncV2)
	if !result.Success {
		return nil, fmt.Errorf("extract transaction failed")
	}

	extData := make(map[string][]*openwallet.TxExtractData)
	for key, data := range result.extractData {
		txs := extData[key]
		if txs == nil {
			txs = make([]*openwallet.TxExtractData, 0)
		}
		txs = append(txs, data)
		extData[key] = txs
	}

	for key, data := range result.extractContractData {
		txs := extData[key]
		if txs == nil {
			txs = make([]*openwallet.TxExtractData, 0)
		}
		txs = append(txs, data)
		extData[key] = txs
	}

	return extData, nil
}

//DropRechargeRecords 清楚钱包的全部充值记录
//func (bs *BTCBlockScanner) DropRechargeRecords(accountID string) error {
//	bs.mu.RLock()
//	defer bs.mu.RUnlock()
//
//	wallet, ok := bs.walletInScanning[accountID]
//	if !ok {
//		errMsg := fmt.Sprintf("accountID: %s wallet is not found", accountID)
//		return errors.New(errMsg)
//	}
//
//	return wallet.DropRecharge()
//}

//DeleteRechargesByHeight 删除某区块高度的充值记录
//func (bs *BTCBlockScanner) DeleteRechargesByHeight(height uint64) error {
//
//	bs.mu.RLock()
//	defer bs.mu.RUnlock()
//
//	for _, wallet := range bs.walletInScanning {
//
//		list, err := wallet.GetRecharges(false, height)
//		if err != nil {
//			return err
//		}
//
//		db, err := wallet.OpenDB()
//		if err != nil {
//			return err
//		}
//
//		tx, err := db.Begin(true)
//		if err != nil {
//			return err
//		}
//
//		for _, r := range list {
//			err = db.DeleteStruct(&r)
//			if err != nil {
//				return err
//			}
//		}
//
//		tx.Commit()
//
//		db.Close()
//	}
//
//	return nil
//}

//SaveTxToWalletDB 保存交易记录到钱包数据库
func (bs *BTCBlockScanner) SaveUnscanRecord(record *openwallet.UnscanRecord) error {

	if bs.BlockchainDAI == nil {
		return fmt.Errorf("Blockchain DAI is not setup ")
	}

	return bs.BlockchainDAI.SaveUnscanRecord(record)
}

//GetWalletByAddress 获取地址对应的钱包
//func (bs *BTCBlockScanner) GetWalletByAddress(address string) (*openwallet.Wallet, bool) {
//	bs.mu.RLock()
//	defer bs.mu.RUnlock()
//
//	account, ok := bs.addressInScanning[address]
//	if ok {
//		wallet, ok := bs.walletInScanning[account]
//		return wallet, ok
//
//	} else {
//		return nil, false
//	}
//}

//GetBlockHeight 获取区块链高度
func (wm *WalletManager) GetBlockHeight() (uint64, error) {

	if wm.Config.RPCServerType == RPCServerExplorer {
		return wm.getBlockHeightByExplorer()
	} else {
		return wm.getBlockHeightByCore()
	}
}

//getBlockHeightByCore 获取区块链高度
func (wm *WalletManager) getBlockHeightByCore() (uint64, error) {

	result, err := wm.WalletClient.Call("getblockcount", nil)
	if err != nil {
		return 0, err
	}

	return result.Uint(), nil
}

//GetLocalNewBlock 获取本地记录的区块高度和hash
func (bs *BTCBlockScanner) GetLocalNewBlock() (uint64, string, error) {

	if bs.BlockchainDAI == nil {
		return 0, "", fmt.Errorf("Blockchain DAI is not setup ")
	}

	header, err := bs.BlockchainDAI.GetCurrentBlockHead(bs.wm.Symbol())
	if err != nil {
		return 0, "", err
	}

	return header.Height, header.Hash, nil
}

//SaveLocalNewBlock 记录区块高度和hash到本地
func (bs *BTCBlockScanner) SaveLocalNewBlock(blockHeight uint64, blockHash string) error {

	if bs.BlockchainDAI == nil {
		return fmt.Errorf("Blockchain DAI is not setup ")
	}

	header := &openwallet.BlockHeader{
		Hash:   blockHash,
		Height: blockHeight,
		Fork:   false,
		Symbol: bs.wm.Symbol(),
	}

	return bs.BlockchainDAI.SaveCurrentBlockHead(header)
}

//SaveLocalBlock 记录本地新区块
func (bs *BTCBlockScanner) SaveLocalBlock(block *Block) error {

	if bs.BlockchainDAI == nil {
		return fmt.Errorf("Blockchain DAI is not setup ")
	}

	header := &openwallet.BlockHeader{
		Hash:              block.Hash,
		Merkleroot:        block.Merkleroot,
		Previousblockhash: block.Previousblockhash,
		Height:            block.Height,
		Time:              uint64(block.Time),
		Symbol:            bs.wm.Symbol(),
	}

	return bs.BlockchainDAI.SaveLocalBlockHead(header)
}

//GetBlockHash 根据区块高度获得区块hash
func (wm *WalletManager) GetBlockHash(height uint64) (string, error) {

	if wm.Config.RPCServerType == RPCServerExplorer {
		return wm.getBlockHashByExplorer(height)
	} else {
		return wm.getBlockHashByCore(height)
	}
}

//getBlockHashByCore 根据区块高度获得区块hash
func (wm *WalletManager) getBlockHashByCore(height uint64) (string, error) {

	request := []interface{}{
		height,
	}

	result, err := wm.WalletClient.Call("getblockhash", request)
	if err != nil {
		return "", err
	}

	return result.String(), nil
}

//GetLocalBlock 获取本地区块数据
func (bs *BTCBlockScanner) GetLocalBlock(height uint64) (*Block, error) {

	if bs.BlockchainDAI == nil {
		return nil, fmt.Errorf("Blockchain DAI is not setup ")
	}

	header, err := bs.BlockchainDAI.GetLocalBlockHeadByHeight(height, bs.wm.Symbol())
	if err != nil {
		return nil, err
	}

	block := &Block{
		Hash:   header.Hash,
		Height: header.Height,
	}

	return block, nil
}

//GetBlock 获取区块数据
func (wm *WalletManager) GetBlock(hash string) (*Block, error) {

	if wm.Config.RPCServerType == RPCServerExplorer {
		return wm.getBlockByExplorer(hash)
	} else {
		return wm.getBlockByCore(hash)
	}
}

//getBlockByCore 获取区块数据
func (wm *WalletManager) getBlockByCore(hash string) (*Block, error) {

	request := []interface{}{
		hash,
	}

	result, err := wm.WalletClient.Call("getblock", request)
	if err != nil {
		return nil, err
	}

	return NewBlock(result), nil
}

//GetTxIDsInMemPool 获取待处理的交易池中的交易单IDs
func (wm *WalletManager) GetTxIDsInMemPool() ([]string, error) {

	if wm.Config.RPCServerType == RPCServerExplorer {
		return wm.getTxIDsInMemPoolByExplorer()
	} else {
		return wm.getTxIDsInMemPoolByCore()
	}
}

//getTxIDsInMemPoolByCore 获取待处理的交易池中的交易单IDs
func (wm *WalletManager) getTxIDsInMemPoolByCore() ([]string, error) {

	var (
		txids = make([]string, 0)
	)

	result, err := wm.WalletClient.Call("getrawmempool", nil)
	if err != nil {
		return nil, err
	}

	if !result.IsArray() {
		return nil, errors.New("no query record")
	}

	for _, txid := range result.Array() {
		txids = append(txids, txid.String())
	}

	return txids, nil
}

//GetTransaction 获取交易单
func (wm *WalletManager) GetTransaction(txid string) (*Transaction, error) {

	//request := []interface{}{
	//	txid,
	//	true,
	//}
	//
	//result, err := wm.WalletClient.Call("getrawtransaction", request)
	//if err != nil {
	//	return nil, err
	//}
	//
	//return result, nil

	if wm.Config.RPCServerType == RPCServerExplorer {
		return wm.getTransactionByExplorer(txid)
	} else {
		return wm.getTransactionByCore(txid)
	}

}

//getTransactionByCore 获取交易单
func (wm *WalletManager) getTransactionByCore(txid string) (*Transaction, error) {

	request := []interface{}{
		txid,
		true,
	}

	result, err := wm.WalletClient.Call("getrawtransaction", request)
	if err != nil {
		return nil, err
	}

	return newTxByCore(result, wm.Config.isTestNet), nil
}

//GetTxOut 获取交易单输出信息，用于追溯交易单输入源头
func (wm *WalletManager) GetTxOut(txid string, vout uint64) (*Vout, error) {

	if wm.Config.RPCServerType == RPCServerExplorer {
		return wm.getTxOutByExplorer(txid, vout)
	} else {
		return wm.getTxOutByCore(txid, vout)
	}
}

//getTxOutByCore 获取交易单输出信息，用于追溯交易单输入源头
func (wm *WalletManager) getTxOutByCore(txid string, vout uint64) (*Vout, error) {

	request := []interface{}{
		txid,
		vout,
	}

	result, err := wm.WalletClient.Call("gettxout", request)
	if err != nil {
		return nil, err
	}

	output := newTxVoutByCore(result, wm.Config.isTestNet)

	/*
		{
			"bestblock": "0000000000012164c0fb1f7ac13462211aaaa83856073bf94faf2ea9c6ea193a",
			"confirmations": 8,
			"value": 0.64467249,
			"scriptPubKey": {
				"asm": "OP_DUP OP_HASH160 dbb494b649a48b22bfd6383dca1712cc401cddde OP_EQUALVERIFY OP_CHECKSIG",
				"hex": "76a914dbb494b649a48b22bfd6383dca1712cc401cddde88ac",
				"reqSigs": 1,
				"type": "pubkeyhash",
				"addresses": ["n1Yec3dmXEW4f8B5iJa5EsspNQ4Ar6K3Ek"]
			},
			"coinbase": false

		}
	*/

	return output, nil

}

//获取未扫记录
func (bs *BTCBlockScanner) GetUnscanRecords() ([]*openwallet.UnscanRecord, error) {
	if bs.BlockchainDAI == nil {
		return nil, fmt.Errorf("Blockchain DAI is not setup ")
	}

	return bs.BlockchainDAI.GetUnscanRecords(bs.wm.Symbol())
}

//DeleteUnscanRecord 删除指定高度的未扫记录
func (bs *BTCBlockScanner) DeleteUnscanRecord(height uint64) error {
	if bs.BlockchainDAI == nil {
		return fmt.Errorf("Blockchain DAI is not setup ")
	}

	return bs.BlockchainDAI.DeleteUnscanRecordByHeight(height, bs.wm.Symbol())
}

//GetAssetsAccountBalanceByAddress 查询账户相关地址的交易记录
func (bs *BTCBlockScanner) GetBalanceByAddress(address ...string) ([]*openwallet.Balance, error) {

	return bs.wm.getBalanceCalUnspent(address...)

}


//getBalanceByExplorer 获取地址余额
func (wm *WalletManager) getBalanceCalUnspent(address ...string) ([]*openwallet.Balance, error) {

	utxos, err := wm.ListUnspent(0, address...)
	if err != nil {
		return nil, err
	}

	addrBalanceMap := wm.calculateUnspent(utxos)
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
func (wm *WalletManager) calculateUnspent(utxos []*Unspent) map[string]*openwallet.Balance {

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

//Run 运行
func (bs *BTCBlockScanner) Run() error {

	//使用浏览器，开启socketIO监听内存池交易
	//if bs.wm.Config.RPCServerType == RPCServerExplorer {
	//	if bs.socketIO == nil {
	//		go bs.setupSocketIO()
	//	}
	//}

	bs.BlockScannerBase.Run()

	return nil
}

////Stop 停止扫描
func (bs *BTCBlockScanner) Stop() error {

	//if bs.socketIO != nil {
	//	bs.socketIO.Close()
	//	bs.socketIO = nil
	//}

	//通知停止线程
	//bs.stopSocketIO <- struct{}{}

	bs.BlockScannerBase.Stop()
	return nil
}

/******************* 使用insight socket.io 监听区块 *******************/

func (bs *BTCBlockScanner) connectSocketIO(disconnected chan struct{}) (*gosocketio.Client, error) {

	var (
		room = "inv"
	)

	apiUrl, err := url.Parse(bs.wm.Config.serverAPI)
	if err != nil {
		return nil, err
	}
	domain := apiUrl.Hostname()
	port := common.NewString(apiUrl.Port()).Int()
	bs.wm.Log.Info("block scanner socketIO connecting")
	socketIO, err := gosocketio.Dial(
		gosocketio.GetUrl(domain, port, false),
		transport.GetDefaultWebsocketTransport())
	if err != nil {
		return nil, err
	}

	err = socketIO.On("tx", func(h *gosocketio.Channel, args interface{}) {
		//bs.wm.Log.Info("block scanner socketIO get new transaction received: ", args)
		txMap, ok := args.(map[string]interface{})
		if ok {
			txid := txMap["txid"].(string)
			//bs.wm.Log.Debugf("new tx: %s", txid)
			errInner := bs.BatchExtractTransaction(0, "", []string{txid})
			if errInner != nil {
				bs.wm.Log.Std.Info("block scanner can not extractRechargeRecords; unexpected error: %v", errInner)
			}
		}

	})
	if err != nil {
		socketIO.Close()
		return nil, err
	}

	err = socketIO.On(gosocketio.OnDisconnection, func(h *gosocketio.Channel) {
		bs.wm.Log.Info("block scanner socketIO disconnected")
		disconnected <- struct{}{}
	})
	if err != nil {
		socketIO.Close()
		return nil, err
	}

	err = socketIO.On(gosocketio.OnConnection, func(h *gosocketio.Channel) {
		bs.wm.Log.Info("block scanner socketIO connected")
		h.Emit("subscribe", room)
	})
	if err != nil {
		socketIO.Close()
		return nil, err
	}
	return socketIO, nil
}

//setupSocketIO 配置socketIO监听新区块
func (bs *BTCBlockScanner) setupSocketIO() error {

	bs.wm.Log.Info("block scanner use socketIO to listen new data")

	var (
		err error
		//连接状态通道
		reconnect = make(chan bool, 1)
		//断开状态通道
		disconnected = make(chan struct{}, 1)
		//重连时的等待时间
		reconnectWait = 5
		socketIO      *gosocketio.Client
	)

	defer func() {
		close(reconnect)
		close(disconnected)
	}()

	//启动连接
	reconnect <- true

	//重连运行时
	for {
		select {
		case <-reconnect:
			//重新连接
			socketIO, err = bs.connectSocketIO(disconnected)
			bs.socketIO = socketIO
			if err != nil {
				bs.wm.Log.Errorf("Connect socketIO failed unexpected error: %v", err)
				disconnected <- struct{}{}
			}

		case <-disconnected:

			if bs.socketIO != nil {
				bs.socketIO.Close()
			}

			//重新连接，前等待
			bs.wm.Log.Info("Auto reconnect after", reconnectWait, "seconds...")
			time.Sleep(time.Duration(reconnectWait) * time.Second)
			reconnect <- true
		case <-bs.stopSocketIO:
			bs.wm.Log.Info("block scanner socketIO has been stopped")
			return nil
		}
	}

	return nil
}

//SupportBlockchainDAI 支持外部设置区块链数据访问接口
//@optional
func (bs *BTCBlockScanner) SupportBlockchainDAI() bool {
	return true
}
