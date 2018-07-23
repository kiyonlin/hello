package api

import (
	"fmt"
	"github.com/pkg/errors"
	"hello/model"
	"hello/util"
	"strings"
	"time"
)

func CancelOrder(market string, symbol string, orderId string) (result bool, errCode, msg string) {
	switch market {
	case model.Huobi:
		return CancelOrderHuobi(orderId)
	case model.OKEX:
		return CancelOrderOkex(symbol, orderId)
	case model.Binance:
		return CancelOrderBinance(symbol, orderId)
	case model.Fcoin:
		return CancelOrderFcoin(orderId)
	case model.Coinpark:
		return CancelOrderCoinpark(orderId)
	case model.Coinbig:
		return CancelOrderCoinbig(orderId)
	}
	return false, `market-not-supported`, `market not supported ` + market
}

func QueryOrderById(market, symbol, orderId string) (dealAmount, dealPrice float64, status string) {
	util.Notice(fmt.Sprintf(`query order %s %s %s`, market, symbol, orderId))
	switch market {
	case model.Huobi:
		dealAmount, dealPrice, status = QueryOrderHuobi(orderId)
	case model.OKEX:
		dealAmount, dealPrice, status = QueryOrderOkex(symbol, orderId)
	case model.Binance:
		dealAmount, dealPrice, status = QueryOrderBinance(symbol, orderId)
	case model.Fcoin:
		dealAmount, dealPrice, status = QueryOrderFcoin(symbol, orderId)
	case model.Coinpark:
		dealAmount,dealPrice, status = QueryOrderCoinpark(orderId)
	case model.Coinbig:
		dealAmount, status = QueryOrderCoinbig(orderId)
	}
	return dealAmount, dealPrice, status
}

func RefreshAccount(market string) {
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

// orderSide: OrderSideBuy OrderSideSell
// orderType: OrderTypeLimit OrderTypeMarket
// amount:如果是限价单或市价卖单，amount是左侧币种的数量，如果是市价买单，amount是右测币种的数量
func PlaceOrder(orderSide, orderType, market, symbol, price, amount string) (orderId, errCode, status string) {
	if strings.Contains(price, `.`) {
		price = strings.TrimRight(price, `0`)
	}
	if strings.Contains(amount, `.`) {
		amount = strings.TrimRight(amount, `0`)
	}
	switch market {
	case model.Huobi:
		orderId, errCode = placeOrderHuobi(orderSide, orderType, symbol, price, amount)
	case model.OKEX:
		orderId, errCode = placeOrderOkex(orderSide, orderType, symbol, price, amount)
	case model.Binance:
		orderId, errCode = placeOrderBinance(orderSide, orderType, symbol, price, amount)
	case model.Fcoin:
		orderId, errCode = placeOrderFcoin(orderSide, orderType, symbol, price, amount)
	case model.Coinpark:
		orderId, errCode, _ = placeOrderCoinpark(orderSide, orderType, symbol, price, amount)
		if errCode == `4003` {
			util.Notice(`【發現4003錯誤】sleep 3 minutes`)
			time.Sleep(time.Minute *3)
		}
	case model.Coinbig:
		orderId, errCode = placeOrderCoinbig(orderSide, orderType, symbol, price, amount)
	}
	if orderId == "0" || orderId == "" {
		status = model.CarryStatusFail
	} else {
		status = model.CarryStatusWorking
	}
	util.Notice(fmt.Sprintf(`[%s-%s] %s %s price: %s amount %s [orderId: %s] errCode %s`, orderSide, orderType,
		market, symbol, price, amount, orderId, errCode))
	return orderId, errCode, status
}

func Order(carry *model.Carry, orderSide, orderType, market, symbol, price string, amount string) {
	carry.DealAskOrderId, carry.DealAskErrCode, carry.DealAskStatus = PlaceOrder(orderSide, orderType, market, symbol,
		price, amount)
	model.InnerCarryChannel <- *carry
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
