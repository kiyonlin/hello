package api

import (
	"time"
	"hello/model"
	"hello/util"
	"strconv"
	"strings"
)

func cancelOrder(market string, symbol string, orderId string) {
	switch market {
	case model.Huobi:
		CancelOrderHuobi(orderId)
	case model.OKEX:
		CancelOrderOkex(symbol, orderId)
	case model.Binance:
		CancelOrderBinance(symbol, orderId)
	}
}

func queryOrder(carry *model.Carry) {
	if carry.DealBidStatus == model.CarryStatusWorking && carry.DealBidOrderId != "" {
		switch carry.BidWeb {
		case model.Huobi:
			carry.DealBidAmount, carry.DealBidStatus = QueryOrderHuobi(carry.DealBidOrderId)
		case model.OKEX:
			carry.DealBidAmount, carry.DealBidStatus = QueryOrderOkex(carry.Symbol, carry.DealBidOrderId)
		case model.Binance:
			carry.DealBidAmount, carry.DealBidStatus = QueryOrderBinance(carry.Symbol, carry.DealBidOrderId)
		}
	}
	if carry.DealAskStatus == model.CarryStatusWorking && carry.DealAskOrderId != "" {
		switch carry.AskWeb {
		case model.Huobi:
			carry.DealAskAmount, carry.DealAskStatus = QueryOrderHuobi(carry.DealAskOrderId)
		case model.OKEX:
			carry.DealAskAmount, carry.DealAskStatus = QueryOrderOkex(carry.Symbol, carry.DealAskOrderId)
		case model.Binance:
			carry.DealAskAmount, carry.DealAskStatus = QueryOrderBinance(carry.Symbol, carry.DealAskOrderId)
		}
	}
	model.CarryChannel <- *carry
}

func CarryProcessor() {
	index := 0
	for true {
		var carry model.Carry
		model.ApplicationDB.Where("deal_bid_status = ? OR deal_ask_status = ?", model.CarryStatusWorking, model.CarryStatusWorking).Offset(index).First(&carry)
		if model.ApplicationDB.NewRecord(&carry) {
			util.SocketInfo("Pause carry processor for 10 minutes")
			time.Sleep(time.Minute * 10)
			index = 0
		} else {
			// cancel order if delay too long (5 minutes)
			if carry.DealBidStatus == model.CarryStatusWorking && util.GetNowUnixMillion()-carry.BidTime > 300000 {
				orderId := carry.DealBidOrderId
				cancelOrder(carry.BidWeb, carry.Symbol, orderId)
			}
			if carry.DealAskStatus == model.CarryStatusWorking && util.GetNowUnixMillion()-carry.AskTime > 300000 {
				orderId := carry.DealAskOrderId
				cancelOrder(carry.AskWeb, carry.Symbol, orderId)
			}
			queryOrder(&carry)
			index++
			time.Sleep(time.Second * 10)
		}
	}
}

var currencyPrice = make(map[string]float64)
var getBuyPriceOkexTime = make(map[string]int64)

func GetBuyPriceOkex(symbol string) (buy float64, err error) {
	if model.ApplicationConfig == nil {
		model.ApplicationConfig = model.NewConfig()
	}
	buy = currencyPrice[symbol]
	now := util.GetNowUnixMillion()
	buy = currencyPrice[symbol]
	if buy != 0 && now-getBuyPriceOkexTime[symbol] < 3000000 {
		return buy, nil
	}
	getBuyPriceOkexTime[symbol] = now
	strs := strings.Split(symbol, "_")
	if strs[0] == strs[1] {
		return 1, nil
	}
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded",
		"User-Agent": "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.71 Safari/537.36"}
	responseBody, _ := util.HttpRequest("GET", model.ApplicationConfig.RestUrls[model.OKEX]+"/ticker.do?symbol="+symbol, "", headers)
	tickerJson, err := util.NewJSON(responseBody)
	if err == nil {
		strBuy, _ := tickerJson.GetPath("ticker", "buy").String()
		buy, err = strconv.ParseFloat(strBuy, 64)
	}
	currencyPrice[symbol] = buy
	return buy, err
}

func AccountDBHandlerServe() {
	for true {
		account := <-model.AccountChannel
		var accountInDb model.Account
		account.PriceInUsdt, _ = GetBuyPriceOkex(account.Currency + "_usdt")
		account.BelongDate = util.GetNow().Format("2006-01-02")
		model.ApplicationDB.Where("market = ? AND currency = ? AND belong_date = ?", account.Market, account.Currency, account.BelongDate).Order("created_at desc").First(&accountInDb)
		if model.ApplicationDB.NewRecord(&accountInDb) {
			model.ApplicationDB.Create(&account)
		} else {
			accountInDb.Free = account.Free
			accountInDb.Frozen = account.Frozen
			model.ApplicationDB.Model(&accountInDb).Updates(map[string]interface{}{"free": account.Free, "frozen": account.Frozen, "price_in_usdt": account.PriceInUsdt})
		}
	}
}

func CarryDBHandlerServe() {
	for true {
		carry := <-model.CarryChannel
		var carryInDb model.Carry
		model.ApplicationDB.Where("bid_time = ? AND ask_time = ?", carry.BidTime, carry.AskTime).First(&carryInDb)
		if model.ApplicationDB.NewRecord(&carryInDb) {
			util.SocketInfo("insert carry" + carryInDb.Symbol)
			model.ApplicationDB.Create(&carry)
		} else {
			util.SocketInfo("update carry")
			if carry.DealAskOrderId != "" {
				carryInDb.DealAskOrderId = carry.DealAskOrderId
			}
			if carry.DealBidOrderId != "" {
				carryInDb.DealBidOrderId = carry.DealBidOrderId
			}
			if carry.DealAskErrCode != "" {
				carryInDb.DealAskErrCode = carry.DealAskErrCode
			}
			if carry.DealBidErrCode != "" {
				carryInDb.DealBidErrCode = carry.DealBidErrCode
			}
			if carry.DealAskAmount != 0 {
				carryInDb.DealAskAmount = carry.DealAskAmount
			}
			if carry.DealBidAmount != 0 {
				carryInDb.DealBidAmount = carry.DealBidAmount
			}
			if carry.DealAskStatus != "" {
				carryInDb.DealAskStatus = carry.DealAskStatus
			}
			if carry.DealBidStatus != "" {
				carryInDb.DealBidStatus = carry.DealBidStatus
			}
			model.ApplicationDB.Save(&carryInDb)
		}
	}
}
