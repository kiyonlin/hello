package main

import (
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
var priceKLine []float64
var totalBuy = 0.0
var countBuy = 0
var totalSell = 0.0
var countSell = 0

const tradeFee = 0.0005

func sell(price float64) {
	diff := coin*price - money
	coin -= diff / 2 / price
	money += (diff / 2) * (1 - tradeFee)
	fmt.Println(fmt.Sprintf(`sell %f  money %f coin %f coin money %f in all %f`,
		price, money, coin, coin*price, money+coin*price))
	lastPrice = price
	totalSell += lastPrice
	countSell++
}

func buy(price float64) {
	diff := money - coin*price
	coin += (diff / 2 / price) * (1 - tradeFee)
	money -= diff / 2
	fmt.Println(fmt.Sprintf(`buy %f  money %f coin %f coin money %f in all %f`,
		price, money, coin, coin*price, money+coin*price))
	lastPrice = price
	totalBuy += lastPrice
	countBuy++
}

func printBalance() {
	fmt.Println(fmt.Sprintf(`buy1 %f sell1 %f count buy1 %d count sell1 %d avg buy1 %f avg sell1 %f`,
		totalBuy, totalSell, countBuy, countSell, totalBuy/float64(countBuy), totalSell/float64(countSell)))
	fmt.Println(fmt.Sprintf(`>>>>>>>>>>>>>>>>>>>>>>>>>>>>>%f`, coin*priceKLine[len(priceKLine)-1]+money))
}
func analyzeKLine(data []interface{}, size int, percentage float64) {
	priceKLine = make([]float64, size)
	for key, value := range data {
		str := value.([]interface{})[4].(string)
		priceKLine[key], _ = strconv.ParseFloat(str, 64)
	}
	lastPrice = priceKLine[0]
	coin = money / lastPrice
	slot := lastPrice * percentage
	fmt.Println(fmt.Sprintf(`buy%f`, lastPrice))
	for i := 2; i < size-1; i++ {
		if priceKLine[i-1] > priceKLine[i-2] && priceKLine[i] < priceKLine[i-1] && priceKLine[i] > priceKLine[i-2] &&
			priceKLine[i]-lastPrice > slot {
			sell(priceKLine[i])
		}
		if priceKLine[i-1] < priceKLine[i-2] && priceKLine[i] > priceKLine[i-1] && priceKLine[i] < priceKLine[i-2] &&
			priceKLine[i]-lastPrice < -1*slot {
			buy(priceKLine[i])
		}
	}
	printBalance()
	money = 5000.0
	coin = 0.0
	lastPrice = priceKLine[0]
	coin = money / lastPrice
	fmt.Println(fmt.Sprintf(`buy%f`, lastPrice))
	for i := 1; i < size-1; i++ {
		if priceKLine[i]-lastPrice > slot {
			sell(priceKLine[i])
		}
		if priceKLine[i]-lastPrice < -1*slot {
			buy(priceKLine[i])
		}
	}
	printBalance()
}

func testApi() {
	model.NewConfig()
	err := configor.Load(model.ApplicationConfig, "./config.yml")
	if err != nil {
		util.Notice(err.Error())
		return
	}
	//model.ApplicationDB, err = gorm.Open("postgres", model.ApplicationConfig.DBConnection)
	//if err != nil {
	//	util.Notice(err.Error())
	//	return
	//}
	//defer model.ApplicationDB.Close()
	//model.LoadSettings()
	//setting := model.ApplicationFutureAccount[model.OKFUTURE][`btc_this_week`]
	size := 2000
	data := api.GetKLineOkex(`btc_usdt`, `30min`, int64(size))
	analyzeKLine(data, size, 0.01)
	//result, errCode := api.FundTransferOkex(`eos_usd`, 21.3711, `3`, `1`)
	//fmt.Println(fmt.Sprintf(`return %t %s`, result, errCode))
	//fmt.Println(fmt.Sprintf(`market %s symbol %s %f %f`, setting.Market, setting.Symbol, setting.OpenedShort, setting.OpenedLong))
	//api.CancelOrder(`binance`, `eos_usdt`, `184201444`)
	//api.CancelOrder(`binance`, `eos_usdt`, `184201445`)
}

func main() {
	testApi()
}
