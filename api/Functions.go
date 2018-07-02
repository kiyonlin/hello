package api

import (
	"github.com/pkg/errors"
	"hello/model"
	"hello/util"
	"strings"
	"time"
)

func RefreshAccounts() {
	for true {
		markets := model.ApplicationConfig.Markets
		for _, value := range markets {
			switch value {
			case model.Huobi:
				model.HuobiAccountId, _ = GetSpotAccountId(model.ApplicationConfig)
				GetAccountHuobi(model.ApplicationAccounts)
			case model.OKEX:
				GetAccountOkex(model.ApplicationAccounts)
			case model.Binance:
				GetAccountBinance(model.ApplicationAccounts)
			case model.Fcoin:
				GetAccountFcoin(model.ApplicationAccounts)
			case model.Coinpark:
				GetAccountCoinpark(model.ApplicationAccounts)

			}
		}
		time.Sleep(time.Second * 20)
	}
}

func DoAsk(carry *model.Carry, price string, amount string) (orderId, errCode string) {
	util.Notice(carry.AskWeb + "ask" + carry.Symbol + " with price: " + price + " amount:" + amount)
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
	}
	carry.DealAskErrCode = errCode
	carry.DealAskOrderId = orderId
	if orderId == "0" || orderId == "" {
		carry.DealAskStatus = model.CarryStatusFail
	} else {
		carry.DealAskStatus = model.CarryStatusWorking
	}
	carry.SideType = `ask`
	model.BidAskChannel <- *carry
	return orderId, errCode
}

func DoBid(carry *model.Carry, price string, amount string) (orderId, errCode string) {

	util.Notice(carry.BidWeb + "bid" + carry.Symbol + " with price: " + price + " amount:" + amount)
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
		orderId, errCode, _ = PlaceOrderCoinpark(carry.Symbol, 1, 1, price, amount)
		GetAccountCoinpark(model.ApplicationAccounts)
	}
	//carry.DealBidAmount, _ = strconv.ParseFloat(amount, 64)
	carry.DealBidErrCode = errCode
	carry.DealBidOrderId = orderId
	if orderId == "0" || orderId == "" {
		carry.DealBidStatus = model.CarryStatusFail
	} else {
		carry.DealBidStatus = model.CarryStatusWorking
	}
	carry.SideType = `bid`
	model.BidAskChannel <- *carry
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
