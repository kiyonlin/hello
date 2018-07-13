package api

import (
	"github.com/pkg/errors"
	"hello/model"
	"hello/util"
	"strings"
	"time"
	"fmt"
)

func CancelOrder(market string, symbol string, orderId string) {
	switch market {
	case model.Huobi:
		CancelOrderHuobi(orderId)
	case model.OKEX:
		CancelOrderOkex(symbol, orderId)
	case model.Binance:
		CancelOrderBinance(symbol, orderId)
	case model.Fcoin:
		CancelOrderFcoin(orderId)
	case model.Coinpark:
		CancelOrderCoinpark(orderId)
	case model.Coinbig:
		CancelOrderCoinbig(orderId)
	}
}

func QueryOrderById(market, symbol, orderId string) (dealAmount float64, status string) {
	switch market {
	case model.Huobi:
		dealAmount, status = QueryOrderHuobi(orderId)
	case model.OKEX:
		dealAmount, status = QueryOrderOkex(symbol, orderId)
	case model.Binance:
		dealAmount, status = QueryOrderBinance(symbol, orderId)
	case model.Fcoin:
		dealAmount, status = QueryOrderFcoin(symbol, orderId)
	case model.Coinpark:
		dealAmount, status = QueryOrderCoinpark(orderId)
	case model.Coinbig:
		dealAmount, status = QueryOrderCoinbig(orderId)
	}
	return dealAmount, status
}

func QueryOrder(carry *model.Carry) {
	if carry.DealBidOrderId != "" && carry.DealBidStatus == model.CarryStatusWorking {
		switch carry.BidWeb {
		case model.Huobi:
			carry.DealBidAmount, carry.DealBidStatus = QueryOrderHuobi(carry.DealBidOrderId)
		case model.OKEX:
			carry.DealBidAmount, carry.DealBidStatus = QueryOrderOkex(carry.Symbol, carry.DealBidOrderId)
		case model.Binance:
			carry.DealBidAmount, carry.DealBidStatus = QueryOrderBinance(carry.Symbol, carry.DealBidOrderId)
		case model.Fcoin:
			carry.DealBidAmount, carry.DealBidStatus = QueryOrderFcoin(carry.Symbol, carry.DealBidOrderId)
		case model.Coinpark:
			carry.DealBidAmount, carry.DealBidStatus = QueryOrderCoinpark(carry.DealBidOrderId)
		case model.Coinbig:
			carry.DealBidAmount, carry.DealBidStatus = QueryOrderCoinbig(carry.DealBidOrderId)
		}
	}
	if carry.DealAskOrderId != "" && carry.DealAskStatus == model.CarryStatusWorking {
		switch carry.AskWeb {
		case model.Huobi:
			carry.DealAskAmount, carry.DealAskStatus = QueryOrderHuobi(carry.DealAskOrderId)
		case model.OKEX:
			carry.DealAskAmount, carry.DealAskStatus = QueryOrderOkex(carry.Symbol, carry.DealAskOrderId)
		case model.Binance:
			carry.DealAskAmount, carry.DealAskStatus = QueryOrderBinance(carry.Symbol, carry.DealAskOrderId)
		case model.Fcoin:
			carry.DealAskAmount, carry.DealAskStatus = QueryOrderFcoin(carry.Symbol, carry.DealAskOrderId)
		case model.Coinpark:
			carry.DealAskAmount, carry.DealAskStatus = QueryOrderCoinpark(carry.DealAskOrderId)
		case model.Coinbig:
			carry.DealAskAmount, carry.DealAskStatus = QueryOrderCoinbig(carry.DealAskOrderId)
		}
	}
	model.CarryChannel <- *carry
}

func RefreshAccounts() {
	for true {
		markets := model.ApplicationConfig.Markets
		for _, value := range markets {
			switch value {
			case model.Huobi:
				GetAccountHuobi(model.ApplicationAccounts)
			case model.OKEX:
				GetAccountOkex(model.ApplicationAccounts)
			case model.Binance:
				GetAccountBinance(model.ApplicationAccounts)
			case model.Fcoin:
				GetAccountFcoin(model.ApplicationAccounts)
			case model.Coinpark:
				GetAccountCoinpark(model.ApplicationAccounts)
			case model.Coinbig:
				GetAccountCoinbig(model.ApplicationAccounts)
			}
		}
		time.Sleep(time.Second * 20)
	}
}

func RefreshAccount(market string)  {
	switch market {
	case model.Huobi:
		GetAccountHuobi(model.ApplicationAccounts)
	case model.OKEX:
		GetAccountOkex(model.ApplicationAccounts)
	case model.Binance:
		GetAccountBinance(model.ApplicationAccounts)
	case model.Fcoin:
		GetAccountFcoin(model.ApplicationAccounts)
	case model.Coinpark:
		GetAccountCoinpark(model.ApplicationAccounts)
	case model.Coinbig:
		GetAccountCoinbig(model.ApplicationAccounts)
	}
}

func SendAsk(market, symbol, price, amount string)(orderId, errCode string)  {
	price = strings.TrimRight(price , `0`)
	amount = strings.TrimRight(amount, `0`)
	switch market {
	case model.Huobi:
		orderId, errCode = PlaceOrderHuobi(symbol, "sell-limit", price, amount)
	case model.OKEX:
		orderId, errCode = PlaceOrderOkex(symbol, "sell", price, amount)
	case model.Binance:
		orderId, errCode = PlaceOrderBinance(symbol, "SELL", price, amount)
	case model.Fcoin:
		orderId, errCode = PlaceOrderFcoin(symbol, "sell", `limit`, price, amount)
	case model.Coinpark:
		orderId, errCode, _ = PlaceOrderCoinpark(symbol, 2, 2, price, amount)
	case model.Coinbig:
		orderId, errCode = PlaceOrderCoinbig(symbol, `sell`, price, amount)
	}
	return orderId, errCode
}

func SendBid(market, symbol, price, amount string)(orderId, errCode string)  {
	price = strings.TrimRight(price , `0`)
	amount = strings.TrimRight(amount, `0`)
	switch market {
	case model.Huobi:
		orderId, errCode = PlaceOrderHuobi(symbol, "buy-limit", price, amount)
	case model.OKEX:
		orderId, errCode = PlaceOrderOkex(symbol, "buy", price, amount)
	case model.Binance:
		orderId, errCode = PlaceOrderBinance(symbol, "BUY", price, amount)
	case model.Fcoin:
		orderId, errCode = PlaceOrderFcoin(symbol, "buy", `limit`, price, amount)
	case model.Coinpark:
		orderId, errCode, _ = PlaceOrderCoinpark(symbol, 1, 2, price, amount)
	case model.Coinbig:
		orderId, errCode = PlaceOrderCoinbig(symbol, `buy`, price, amount)
	}
	return orderId, errCode
}

func DoAsk(carry *model.Carry, price string, amount string) (orderId, errCode string) {
	price = strings.TrimRight(price , `0`)
	amount = strings.TrimRight(amount, `0`)
	switch carry.AskWeb {
	case model.Huobi:
		orderId, errCode = PlaceOrderHuobi(carry.Symbol, "sell-limit", price, amount)
		GetAccountHuobi(model.ApplicationAccounts)
	case model.OKEX:
		orderId, errCode = PlaceOrderOkex(carry.Symbol, "sell", price, amount)
		GetAccountOkex(model.ApplicationAccounts)
	case model.Binance:
		orderId, errCode = PlaceOrderBinance(carry.Symbol, "SELL", price, amount)
		GetAccountBinance(model.ApplicationAccounts)
	case model.Fcoin:
		orderId, errCode = PlaceOrderFcoin(carry.Symbol, "sell", `limit`, price, amount)
		GetAccountFcoin(model.ApplicationAccounts)
	case model.Coinpark:
		orderId, errCode, _ = PlaceOrderCoinpark(carry.Symbol, 2, 2, price, amount)
		GetAccountCoinpark(model.ApplicationAccounts)
	case model.Coinbig:
		orderId, errCode = PlaceOrderCoinbig(carry.Symbol, `sell`, price, amount)
		GetAccountCoinbig(model.ApplicationAccounts)
	}
	carry.DealAskErrCode = errCode
	carry.DealAskOrderId = orderId
	if orderId == "0" || orderId == "" {
		carry.DealAskStatus = model.CarryStatusFail
	} else {
		carry.DealAskStatus = model.CarryStatusWorking
	}
	if carry.SideType == `` {
		carry.SideType = `ask`
	}
	model.BidAskChannel <- *carry
	util.Notice(fmt.Sprintf(`%s ask %s price: %s amount %s [orderId: %s] errCode %s`,
		carry.AskWeb ,carry.Symbol, price, amount, orderId, errCode))
	return orderId, errCode
}

func DoBid(carry *model.Carry, price string, amount string) (orderId, errCode string) {
	price = strings.TrimRight(price , `0`)
	amount = strings.TrimRight(amount, `0`)
	switch carry.BidWeb {
	case model.Huobi:
		orderId, errCode = PlaceOrderHuobi(carry.Symbol, "buy-limit", price, amount)
		GetAccountHuobi(model.ApplicationAccounts)
	case model.OKEX:
		orderId, errCode = PlaceOrderOkex(carry.Symbol, "buy", price, amount)
		GetAccountOkex(model.ApplicationAccounts)
	case model.Binance:
		orderId, errCode = PlaceOrderBinance(carry.Symbol, "BUY", price, amount)
		GetAccountBinance(model.ApplicationAccounts)
	case model.Fcoin:
		orderId, errCode = PlaceOrderFcoin(carry.Symbol, "buy", `limit`, price, amount)
		GetAccountFcoin(model.ApplicationAccounts)
	case model.Coinpark:
		orderId, errCode, _ = PlaceOrderCoinpark(carry.Symbol, 1, 2, price, amount)
		GetAccountCoinpark(model.ApplicationAccounts)
	case model.Coinbig:
		orderId, errCode = PlaceOrderCoinbig(carry.Symbol, `buy`, price, amount)
		GetAccountCoinbig(model.ApplicationAccounts)
	}
	carry.DealBidErrCode = errCode
	carry.DealBidOrderId = orderId
	if orderId == "0" || orderId == "" {
		carry.DealBidStatus = model.CarryStatusFail
	} else {
		carry.DealBidStatus = model.CarryStatusWorking
	}
	if carry.SideType == `` {
		carry.SideType = `bid`
	}
	model.BidAskChannel <- *carry
	util.Notice(fmt.Sprintf(`%s bid %s price: %s amount %s [orderId: %s] errCode %s`,
		carry.BidWeb ,carry.Symbol, price, amount, orderId, errCode))
	return orderId, errCode
}

func Maintain(accounts *model.Accounts, marketName string) {
	accounts.Lock.Lock()
	defer accounts.Lock.Unlock()
	if accounts.Data[marketName] == nil {
		return
	}
	accounts.MarketTotal[marketName] = 0
	for key, value := range accounts.Data[marketName] {
		value.PriceInUsdt, _ = GetPrice(key + "_usdt")
		//util.Info(fmt.Sprintf(`%s price %f`, key, value.PriceInUsdt))
		accounts.MarketTotal[marketName] += value.PriceInUsdt * (value.Free + value.Frozen)
	}
	if accounts.MarketTotal[marketName] == 0 {
		util.Notice(marketName + " balance is empty!!!!!!!!!!!")
		accounts.MarketTotal[marketName] = 1
	}
	for _, value := range accounts.Data[marketName] {
		value.Percentage = value.PriceInUsdt * (value.Free + value.Frozen) / accounts.MarketTotal[marketName]
	}
	// calculate currency percentage of all markets
	accounts.TotalInUsdt = 0
	for _, value := range accounts.MarketTotal {
		accounts.TotalInUsdt += value
	}
	accounts.CurrencyTotal = make(map[string]float64)
	for _, currencies := range accounts.Data {
		for currency, account := range currencies {
			accounts.CurrencyTotal[currency] += (account.Free + account.Frozen) * account.PriceInUsdt
		}
	}
	accounts.CurrencyPercentage = make(map[string]float64)
	for currency, value := range accounts.CurrencyTotal {
		accounts.CurrencyPercentage[currency] = value / accounts.TotalInUsdt
	}
	model.AccountChannel <- accounts.Data[marketName]
}

func GetPrice(symbol string) (buy float64, err error) {
	if model.ApplicationConfig == nil {
		model.NewConfig()
	}
	symbol = strings.TrimSpace(strings.ToLower(symbol))
	strs := strings.Split(symbol, "_")
	if len(strs) != 2 {
		return 0, errors.New(`wrong symbol ` + symbol)
	}
	strs[0] = strings.TrimSpace(strs[0])
	strs[1] = strings.TrimSpace(strs[1])
	if strs[0] == strs[1] {
		return 1, nil
	}
	if model.GetBuyPriceTime[symbol] != 0 && util.GetNowUnixMillion()-model.GetBuyPriceTime[symbol] < 3600000 {
		return model.CurrencyPrice[symbol], nil
	}
	model.GetBuyPriceTime[symbol] = util.GetNowUnixMillion()
	if strs[0] == `ft` || strs[1] == `ft` || model.ApplicationConfig.InChina == 1 {
		return getBuyPriceFcoin(symbol)
	}
	if strings.ToUpper(strs[0]) == `BIX` || strings.ToUpper(strs[1]) == `BIX` {
		return getBuyPriceCoinpark(symbol)
	}
	return getBuyPriceOkex(symbol)
}
