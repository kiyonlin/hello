package main

import (
	"encoding/json"
	"fmt"
	"github.com/jinzhu/configor"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"hello/api"
	"hello/model"
	"hello/util"
	"strconv"
)

var lastPrice float64
var money = 5000.0
var coin = 0.0
var priceKLine []KLinePoint
var totalBuy = 0.0
var countBuy = 0
var totalSell = 0.0
var countSell = 0
var balance = 10000.0

const tradeFee = 0.0003

type KLinePoint struct {
	TS        int64
	EndPrice  float64
	HighPrice float64
	LowPrice  float64
}

func initMoney() {
	money = 5000
	balance = 10000
	coin = 5000 / priceKLine[0].EndPrice
	lastPrice = priceKLine[0].EndPrice
	//fmt.Println(fmt.Sprintf(`buy%f`, lastPrice))
}

func sell(_ KLinePoint, price float64) {
	diff := coin*price - money
	coin -= diff / 2 / price
	money += (diff / 2) * (1 - tradeFee)
	balance = money + coin*price
	//strTime := time.Unix(klPoint.TS/1000, 0).Format("2006-01-02 15:04:05")
	//fmt.Println(fmt.Sprintf(`%s sell %f  money %f coin %f coin money %f in all %f`,
	//	strTime, price, money, coin, coin*price, balance))
	lastPrice = price
	totalSell += lastPrice
	countSell++
}

func buy(_ KLinePoint, price float64) {
	diff := money - coin*price
	coin += (diff / 2 / price) * (1 - tradeFee)
	money -= diff / 2
	balance = money + coin*price
	//strTime := time.Unix(klPoint.TS/1000, 0).Format("2006-01-02 15:04:05")
	//fmt.Println(fmt.Sprintf(`%s buy %f  money %f coin %f coin money %f in all %f`,
	//	strTime, price, money, coin, coin*price, balance))
	lastPrice = price
	totalBuy += lastPrice
	countBuy++
}

func printBalance() {
	//fmt.Println(fmt.Sprintf(`buy1 %f sell1 %f count buy1 %d count sell1 %d avg buy1 %f avg sell1 %f`,
	//	totalBuy, totalSell, countBuy, countSell, totalBuy/float64(countBuy), totalSell/float64(countSell)))
	//fmt.Println(fmt.Sprintf(`条数%d 净值%f`, len(priceKLine), coin*priceKLine[0].EndPrice+money))
}

func analyzeKLine(data []interface{}, percentage float64) {
	priceKLine = make([]KLinePoint, len(data))
	for key, value := range data {
		ts, _ := value.([]interface{})[0].(json.Number).Int64()
		str := value.([]interface{})[4].(string)
		strHigh := value.([]interface{})[2].(string)
		strLow := value.([]interface{})[3].(string)
		price, _ := strconv.ParseFloat(str, 64)
		high, _ := strconv.ParseFloat(strHigh, 64)
		low, _ := strconv.ParseFloat(strLow, 64)
		klinePoint := KLinePoint{TS: ts, EndPrice: price, HighPrice: high, LowPrice: low}
		priceKLine[key] = klinePoint
	}
	//initMoney()
	//for i := 2; i < len(data)-1; i++ {
	//	if priceKLine[i-1].EndPrice > priceKLine[i-2].EndPrice && priceKLine[i].EndPrice < priceKLine[i-1].EndPrice &&
	//		priceKLine[i].EndPrice > priceKLine[i-2].EndPrice && (priceKLine[i].EndPrice-lastPrice)*coin/balance > percentage {
	//		sell(priceKLine[i])
	//	}
	//	if priceKLine[i-1].EndPrice < priceKLine[i-2].EndPrice && priceKLine[i].EndPrice > priceKLine[i-1].EndPrice &&
	//		priceKLine[i].EndPrice < priceKLine[i-2].EndPrice &&
	//		(lastPrice-priceKLine[i].EndPrice)*coin/balance > percentage {
	//		buy(priceKLine[i])
	//	}
	//}
	//printBalance()
	initMoney()
	for i := 1; i < len(data)-1; i++ {
		if (priceKLine[i].HighPrice-lastPrice)*coin/balance > percentage {
			sell(priceKLine[i], percentage*balance/coin+lastPrice)
		}
		if (lastPrice-priceKLine[i].LowPrice)*coin/balance > percentage {
			buy(priceKLine[i], lastPrice-percentage*balance/coin)
		}
	}
	printBalance()
}

func testApi() {
	model.NewConfig()
	err := configor.Load(model.AppConfig, "./config.yml")
	if err != nil {
		util.Notice(err.Error())
		return
	}
	//model.AppDB, err = gorm.Open("postgres", model.AppConfig.DBConnection)
	//if err != nil {
	//	util.Notice(err.Error())
	//	return
	//}
	//defer model.AppDB.Close()
	//model.LoadSettings()
	//setting := model.AppFutureAccount[model.OKFUTURE][`btc_this_week`]
	size := 2000
	//symbols := []string{`btc_usdt`, `eth_usdt`, `eos_usdt`}
	symbols := []string{`eth_btc`, `eth_usdt`, `eos_usdt`, `eos_eth`, `eos_btc`}
	slots := []string{`1min`, `5min`,}
	percentages := []float64{0.0005, 0.001, 0.002, 0.003, 0.004, 0.005, 0.01}
	results := make(map[string]map[float64]map[string]float64)
	for _, slot := range slots {
		fmt.Print("\n" + slot)
		results[slot] = make(map[float64]map[string]float64)
		for _, percentage := range percentages {
			results[slot][percentage] = make(map[string]float64)
			fmt.Print(fmt.Sprintf("\n %f", percentage))
			for _, symbol := range symbols {
				data := api.GetKLineOkex(symbol, slot, int64(size))
				analyzeKLine(data, percentage)
				results[slot][percentage][symbol] = coin*priceKLine[0].EndPrice + money
				fmt.Print(fmt.Sprintf(`	%s:%f `, symbol, results[slot][percentage][symbol]))
			}
		}
	}
	fmt.Println()
	// 5min eth 0.009> eos 0.007 > btc 0.0018
	// 1min btc 净值10004 eth 净值10008 eos 净值10005
	// 30min eos 净值10638.965867
	//result, errCode := api.FundTransferOkex(`eos_usd`, 21.3711, `3`, `1`)
	//fmt.Println(fmt.Sprintf(`return %t %s`, result, errCode))
	//fmt.Println(fmt.Sprintf(`market %s symbol %s %f %f`, setting.Market, setting.Symbol, setting.OpenedShort, setting.OpenedLong))
	//api.CancelOrder(`binance`, `eos_usdt`, `184201444`)
	//api.CancelOrder(`binance`, `eos_usdt`, `184201445`)
}

func _() {
	testApi()
}
