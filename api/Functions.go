package api

import (
	"fmt"
	"github.com/pkg/errors"
	"hello/model"
	"hello/util"
	"strconv"
	"strings"
	"time"
)

// 根据不同的网站返回价格小数位
func GetPriceDecimal(market, symbol string) int {
	switch market {
	case model.Fcoin:
		switch symbol {
		case `ft_usdt`:
			return 6
		case `ft_eth`, `ft_btc`:
			return 8
		case `eth_usdt`, `btc_usdt`:
			return 2
		case `eos_usdt`:
			return 4
		}
	case model.Coinpark:
		switch symbol {
		case `cp_usdt`:
			return 4
		case `cp_eth`, `cp_btc`:
			return 8
		}
	}
	return 8
}

func CancelOrder(market string, symbol string, orderId string) (result bool, errCode, msg string) {
	switch market {
	case model.Huobi:
		return CancelOrderHuobi(orderId)
	case model.OKEX:
		return CancelOrderOkex(symbol, orderId)
	case model.OKFUTURE:
		return CancelOrderOkfuture(symbol, orderId)
	case model.Binance:
		return CancelOrderBinance(symbol, orderId)
	case model.Fcoin:
		return CancelOrderFcoin(orderId)
	case model.Coinpark:
		return CancelOrderCoinpark(orderId)
	case model.Coinbig:
		return CancelOrderCoinbig(orderId)
	case model.Bitmex:
		return CancelOrderBitmex(orderId)
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
	case model.OKFUTURE:
		dealAmount, dealPrice, status = QueryOrderOkfuture(symbol, orderId)
	case model.Binance:
		dealAmount, dealPrice, status = QueryOrderBinance(symbol, orderId)
	case model.Fcoin:
		dealAmount, dealPrice, status = QueryOrderFcoin(symbol, orderId)
	case model.Coinpark:
		dealAmount, dealPrice, status = QueryOrderCoinpark(orderId)
	case model.Coinbig:
		dealAmount, status = QueryOrderCoinbig(orderId)
	case model.Bitmex:
		dealAmount, dealPrice, status = QueryOrderBitmex(orderId)
	}
	return dealAmount, dealPrice, status
}

func RefreshAccount(market string) {
	accounts := model.ApplicationAccounts.GetAccounts(market)
	model.ApplicationAccounts.ClearAccounts(market)
	switch market {
	case model.Huobi:
		getAccountHuobi(model.ApplicationAccounts)
		if accounts != nil {
			same := true
			for key, value := range accounts {
				if value != model.ApplicationAccounts.GetAccount(market, key) {
					same = false
					break
				}
			}
			if same {
				time.Sleep(time.Second * 15)
				getAccountHuobi(model.ApplicationAccounts)
			}
		}
	case model.OKEX:
		getAccountOkex(model.ApplicationAccounts)
	case model.OKFUTURE:
		getAccountOkfuture(model.ApplicationAccounts)
	case model.Binance:
		getAccountBinance(model.ApplicationAccounts)
		if model.ApplicationConfig.BnbMin > 0 && model.ApplicationConfig.BnbBuy > 0 {
			account := model.ApplicationAccounts.GetAccount(model.Binance, `bnb`)
			if account != nil && account.Free < model.ApplicationConfig.BnbMin {
				util.Notice(fmt.Sprintf(`[bnb數量不足]%f - %f`, account.Free, model.ApplicationConfig.BnbMin))
				PlaceOrder(model.OrderSideBuy, model.OrderTypeMarket, model.Binance, `bnb_usdt`,
					0, model.ApplicationConfig.BnbBuy)
			}
		}

	case model.Fcoin:
		getAccountFcoin(model.ApplicationAccounts)
	case model.Coinpark:
		getAccountCoinpark(model.ApplicationAccounts)
	case model.Coinbig:
		getAccountCoinbig(model.ApplicationAccounts)
	case model.Bitmex:
		getAccountBitmex(model.ApplicationAccounts)
	}
	Maintain(model.ApplicationAccounts, market)
}

// orderSide: OrderSideBuy OrderSideSell
// orderType: OrderTypeLimit OrderTypeMarket
// amount:如果是限价单或市价卖单，amount是左侧币种的数量，如果是市价买单，amount是右测币种的数量
func PlaceOrder(orderSide, orderType, market, symbol string, price, amount float64) (orderId, errCode, status string) {
	precision := GetPriceDecimal(market, symbol)
	strPrice := strconv.FormatFloat(price, 'f', precision, 64)
	strAmount := strconv.FormatFloat(amount, 'f', 2, 64)
	switch market {
	case model.Huobi:
		orderId, errCode = placeOrderHuobi(orderSide, orderType, symbol, strPrice, strAmount)
	case model.OKEX:
		orderId, errCode = placeOrderOkex(orderSide, orderType, symbol, strPrice, strAmount)
	case model.OKFUTURE:
		orderId, errCode = placeOrderOkfuture(orderSide, orderType, symbol, strPrice, strAmount)
	case model.Binance:
		orderId, errCode = placeOrderBinance(orderSide, orderType, symbol, strPrice, strAmount)
	case model.Fcoin:
		orderId, errCode = placeOrderFcoin(orderSide, orderType, symbol, strPrice, strAmount)
	case model.Coinpark:
		orderId, errCode, _ = placeOrderCoinpark(orderSide, orderType, symbol, strPrice, strAmount)
		if errCode == `4003` {
			util.Notice(`【發現4003錯誤】sleep 3 minutes`)
			time.Sleep(time.Minute * 3)
		}
	case model.Coinbig:
		orderId, errCode = placeOrderCoinbig(orderSide, orderType, symbol, strPrice, strAmount)
	case model.Bitmex:
		orderId, errCode = placeOrderBitmex(orderSide, orderType, symbol, strPrice, strAmount)
	}
	if orderId == "0" || orderId == "" {
		status = model.CarryStatusFail
	} else {
		status = model.CarryStatusWorking
	}
	util.Notice(fmt.Sprintf(`[%s-%s] %s %s price: %f amount %f [orderId: %s] errCode %s`, orderSide, orderType,
		market, symbol, price, amount, orderId, errCode))
	return orderId, errCode, status
}

func Order(carry *model.Carry, orderSide, orderType, market, symbol string, price, amount float64) {
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
	strs := strings.Split(symbol, "_")
	if len(strs) != 2 {
		return 0, errors.New(`wrong symbol ` + symbol)
	}
	strs[0] = strings.ToUpper(strings.TrimSpace(strs[0]))
	strs[1] = strings.ToUpper(strings.TrimSpace(strs[1]))
	if strs[0] == strs[1] {
		return 1, nil
	}
	symbol = strings.TrimSpace(strings.ToLower(symbol))
	for _, bidAsks := range model.ApplicationMarkets.BidAsks[symbol] {
		if bidAsks.Bids != nil {
			return bidAsks.Bids[0].Price, nil
		}
	}
	if model.GetBuyPriceTime[symbol] != 0 && util.GetNowUnixMillion()-model.GetBuyPriceTime[symbol] < 3600000 {
		return model.CurrencyPrice[symbol], nil
	}
	model.GetBuyPriceTime[symbol] = util.GetNowUnixMillion()
	if strs[0] == `BIX` || strs[1] == `BIX` || strs[0] == `CP` || strs[1] == `CP` {
		return getBuyPriceCoinpark(symbol)
	}
	if strs[0] == `FT` || strs[1] == `FT` || model.ApplicationConfig.InChina == 1 {
		return getBuyPriceFcoin(symbol)
	}
	return getBuyPriceOkex(symbol)
}

func CheckOrderValue(currency string, amount float64) (result bool) {
	currencyPrice, _ := GetPrice(currency + `_usdt`)
	if currencyPrice*amount < model.ApplicationConfig.MinUsdt {
		util.Notice(fmt.Sprintf(`%s下单数量%f不足%f usdt`, currency, amount, model.ApplicationConfig.MinUsdt))
		return false
	}
	return true
}
