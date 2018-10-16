package main

import (
	"github.com/ybbus/jsonrpc"
	"encoding/json"
	"strconv"
	"math"
	"strings"
	"fmt"
	"github.com/kataras/iris/core/errors"
	"github.com/mutalisk999/bitcoin-lib/src/transaction"
	"encoding/hex"
	"bytes"
	"io"
)

type UTXODetail struct {
	TxId          string `json:"txid"`
	Vout          int    `json:"vout"`
	Address       string `json:"address"`
	Account       string `json:"account"`
	ScriptPubKey  string `json:"scriptPubKey"`
	RedeemScript  string `json:"redeemScript"`
	Amount        int64  `json:"amount"`
	Confirmations int    `json:"confirmations"`
	Spendable     bool   `json:"spendable"`
	Solvable      bool   `json:"solvable"`
}
type UTXOsDetail []UTXODetail


var toBalance = int64(100000000)
var feeRate = int64(10000)
var scriptHex = "76a914bb2816d6c6c57a155b8c78d42cda62c615f7c8fc88ac"
var address = "31p5cQ4cxZPuQxUcfL77J4uKxpWcm7JT63" //"1J4bPKYg1cGZkJWxhGCqzXYqgCBviQjKUJ"

var serverUrl = "http://zzt:909090@127.0.0.1:10010"


func ToPrecisionAmount(value string, nPrecision int) (int64, error) {
	precision := int64(math.Pow10(nPrecision))
	strArray := strings.Split(value, ".")
	if len(strArray) == 1 {
		quotient, err := strconv.Atoi(strArray[0])
		if err != nil {
			return 0, errors.New("invalid value: invalid quotient part")
		}
		return int64(quotient) * precision, nil
	} else if len(strArray) == 2 {
		quotient, err := strconv.Atoi(strArray[0])
		if err != nil {
			return 0, errors.New("invalid value: invalid quotient part")
		}

		remainderStr := strArray[1]
		for i:= len(remainderStr); i < nPrecision; i++ {
			remainderStr = remainderStr + "0"
		}
		remainderStr = remainderStr[0:nPrecision]

		remainder, err := strconv.Atoi(remainderStr)
		if err != nil {
			return 0, errors.New("invalid value: invalid remainder part")
		}
		return int64(quotient) * precision + int64(remainder), nil
	} else {
		return 0, errors.New("invalid value: too many point")
	}
}

func FromPrecisionAmount(amount int64, nPrecision int) string {
	precision := int64(math.Pow10(nPrecision))
	quotient := amount / precision
	remainder := amount % precision
	return fmt.Sprintf("%d.%08d", quotient, remainder)
}

func DoHttpJsonRpcCallType1(method string, args ...interface{}) (*jsonrpc.RPCResponse, error) {
	rpcClient := jsonrpc.NewClient(serverUrl)
	rpcResponse, err := rpcClient.Call(method, args)
	if err != nil {
		return nil, err
	}
	return rpcResponse, nil
}


func GetUtxosByAddressRPC(addr string) ([]UTXODetail, error) {
	nPrec := 8

	res, err := DoHttpJsonRpcCallType1("listunspent", 0, 999999, []string{addr})
	if err != nil {
		return nil, err
	}

	var utxos UTXOsDetail
	for _, i := range res.Result.([]interface{}) {
		var utxo UTXODetail
		out := i.(map[string]interface{})

		amount, ok := out["amount"]
		if ok == false {
			continue
		}
		txid, ok := out["txid"]
		if ok == false {
			continue
		}
		vout, ok := out["vout"]
		if ok == false {
			continue
		}
		scriptPubKey, ok := out["scriptPubKey"]
		if ok == false {
			continue
		}
		confirmations, ok := out["confirmations"]
		if ok == false {
			continue
		}

		amountValue, err := amount.(json.Number).Float64()
		if err != nil {
			continue
		}
		amountStr := strconv.FormatFloat(amountValue, 'f', nPrec, 64)
		amountPrec, err := ToPrecisionAmount(amountStr, nPrec)
		if err != nil {
			continue
		}

		if amountPrec == 0 {
			continue
		}
		utxo.Amount = amountPrec

		txidValue := txid.(string)
		utxo.TxId = txidValue

		i64, err := vout.(json.Number).Int64()
		if err != nil {
			continue
		}
		utxo.Vout = int(i64)

		scriptPubKeyValue := scriptPubKey.(string)
		utxo.ScriptPubKey = scriptPubKeyValue

		i64, err = confirmations.(json.Number).Int64()
		if err != nil {
			continue
		}
		utxo.Confirmations = int(i64)

		utxos = append(utxos, utxo)
	}

	return utxos, nil
}

func SignRawTrx(rawTrx string) (string, error) {
	res, err := DoHttpJsonRpcCallType1("signrawtransaction", rawTrx)
	if err != nil {
		return "", err
	}

	result := res.Result.(map[string]interface{})
	signedTrxIf, _ := result["hex"]
	signedTrx := signedTrxIf.(string)
	return signedTrx, nil
}

func SendRawTrx(rawTrx string) (string, error) {
	res, err := DoHttpJsonRpcCallType1("sendrawtransaction", rawTrx)
	if err != nil {
		return "", err
	}

	trxId := res.Result.(string)
	return trxId, nil
}

func GetOneUtxo(utxos []UTXODetail) *UTXODetail {
	for _, utxo := range utxos {
		if utxo.Amount / toBalance > 1 {
			fmt.Println(utxo.Amount)
			return &utxo
		}
	}
	return nil
}

func main() {
	// for {
		utxos, _ := GetUtxosByAddressRPC(address)

		utxo := GetOneUtxo(utxos)
		if utxo == nil {
			fmt.Println("no utxo need to unpick")
			return
		} else {
			fmt.Println("use utxo:", utxo)
		}

		voutCount := utxo.Amount / toBalance
		trxBytesCount := 180 + voutCount*40
		if trxBytesCount > 100*1000 {
			fmt.Println("too large trx size")
			return
		}
		feeCost := int64(float64(trxBytesCount) / 1000.0 * float64(feeRate))

		voutBalance := utxo.Amount - feeCost

		var trx transaction.Transaction
		trx.Version = 2
		trx.LockTime = uint32(0xFFFFFFFF)

		var vin transaction.TxIn
		vin.Sequence = uint32(0xFFFFFFFF)
		vin.PrevOut.Hash.SetHex(utxo.TxId)
		vin.PrevOut.N = uint32(utxo.Vout)

		trx.Vin = append(trx.Vin, vin)

		for {
			var vout transaction.TxOut
			scriptBytes, _ := hex.DecodeString(scriptHex)
			vout.ScriptPubKey.SetScriptBytes(scriptBytes)
			if voutBalance/toBalance <= 1 {
				vout.Value = voutBalance
				trx.Vout = append(trx.Vout, vout)
				break
			} else {
				vout.Value = toBalance
				trx.Vout = append(trx.Vout, vout)
			}
			voutBalance -= toBalance
		}

		bytesBuf := bytes.NewBuffer([]byte{})
		bufWriter := io.Writer(bytesBuf)
		err := trx.Pack(bufWriter)
		if err != nil {
			fmt.Println("pack trx error")
			return
		}

		trxHex := hex.EncodeToString(bytesBuf.Bytes())
		fmt.Println(trxHex)
		// trxSignedHex, err := SignRawTrx(trxHex)
		// if err != nil {
		// 	fmt.Println("sign trx error")
		// 	return
		// }
		// fmt.Println(trxSignedHex)

		// trxId, err := SendRawTrx(trxSignedHex)
		// if err != nil {
		// 	fmt.Println("send trx error")
		// 	return
		// }
		// fmt.Println(trxId)
	// }
}


