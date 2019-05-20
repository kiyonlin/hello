package api

import (
	"fmt"
	"github.com/pkg/errors"
	"hello/model"
	"hello/util"
	"math"
	"strconv"
	"strings"
	"time"
)

// 根据不同的网站返回价格小数位
func GetPriceDecimal(market, symbol string) int {
	switch market {
	case model.Fcoin:
		//{"status":3022,"msg":"limit price decimal: 5"}
		switch symbol {
		case `btc_usdt`, `bch_usdt`, `btc_pax`, `dash_usdt`:
			return 1
		case `eth_usdt`, `ltc_usdt`, `zec_usdt`, `bsv_usdt`:
			return 2
		case `eos_usdt`, `etc_usdt`:
			return 3
		case `ft_usdt`, `xrp_usdt`, `iota_usdt`, `ltc_eth`, `xlm_usdt`:
			return 4
		case `eth_btc`, `eos_eth`, `ltc_btc`, `bch_btc`, `etc_eth`, `zec_btc`:
			return 5
		case `etc_btc`, `xrp_eth`, `iota_eth`:
			return 6
		case `eos_btc`:
			return 7
		case `ft_btc`, `xrp_btc`, `iota_btc`, `ft_eth`:
			return 8
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

func GetAmountDecimal(market, symbol string) int {
	switch market {
	case model.OKEX:
		switch symbol {
		case `eos_usdt`, `btc_usdt`:
			return 4
		}
	case model.Fcoin:
		//{"status":3006,"msg":"limit amount decimal: 2"}
		switch symbol {
		case `btc_usdt`, `btc_pax`, `eos_usdt`, `eth_btc`, `eth_usdt`, `ltc_usdt`, `ltc_btc`, `ltc_eth`,
			`etc_usdt`, `etc_btc`, `etc_eth`, `bch_btc`, `bch_usdt`, `bsv_usdt`, `zec_usdt`, `xlm_usdt`:
			return 4
		case `eos_btc`, `xrp_usdt`, `eos_eth`, `iota_usdt`, `ft_usdt`, `ft_btc`, `ft_eth`:
			return 2
		case `xrp_btc`, `xrp_eth`, `iota_btc`, `iota_eth`:
			return 0
		}
	}
	return 4
}

func MustCancel(market, symbol, orderId string, mustCancel bool) {
	for i := 0; i < 5; i++ {
		result, errCode, _ := CancelOrder(market, symbol, orderId)
		util.Notice(fmt.Sprintf(`[cancel] %s for %d times, return %t `, orderId, i, result))
		if result || !mustCancel { //3008:"submit cancel invalid order state
			break
		} else if errCode == `429` || errCode == `4003` {
			util.Notice(`调用次数繁忙`)
		}
		time.Sleep(time.Millisecond * 500)
	}
}

func CancelOrder(market string, symbol string, orderId string) (result bool, errCode, msg string) {
	errCode = `market-not-supported ` + market
	msg = `market not supported ` + market
	switch market {
	case model.Huobi:
		result, errCode, msg = CancelOrderHuobi(orderId)
	case model.OKEX:
		result, errCode, msg = CancelOrderOkex(symbol, orderId)
	case model.OKFUTURE:
		result, errCode, msg = CancelOrderOkfuture(symbol, orderId)
	case model.Binance:
		result, errCode, msg = CancelOrderBinance(symbol, orderId)
	case model.Fcoin:
		result, errCode, msg = CancelOrderFcoin(orderId)
	case model.Coinpark:
		result, errCode, msg = CancelOrderCoinpark(orderId)
	case model.Coinbig:
		result, errCode, msg = CancelOrderCoinbig(orderId)
	case model.Bitmex:
		result, errCode, msg = CancelOrderBitmex(orderId)
	}
	util.Notice(fmt.Sprintf(`[cancel %s %v %s %s]`, orderId, result, market, symbol))
	return result, errCode, msg
}

func QueryOrders(market, symbol, states string, before, after int64) (orders []*model.Order) {
	switch market {
	case model.Fcoin:
		normal := queryOrdersFcoin(symbol, states, ``, before, after)
		lever := queryOrdersFcoin(symbol, states, model.AccountTypeLever, before, after)
		for _, value := range normal {
			lever = append(lever, value)
		}
		return lever
	default:
		util.Notice(market + ` not supported`)
	}
	return nil
}

func QueryOrderById(market, symbol, orderId string) (order *model.Order) {
	var dealAmount, dealPrice float64
	var status string
	switch market {
	case model.Huobi:
		dealAmount, dealPrice, status = queryOrderHuobi(orderId)
	case model.OKEX:
		dealAmount, dealPrice, status = queryOrderOkex(symbol, orderId)
	case model.OKFUTURE:
		dealAmount, dealPrice, status = queryOrderOkfuture(symbol, orderId)
	case model.Binance:
		dealAmount, dealPrice, status = queryOrderBinance(symbol, orderId)
	case model.Fcoin:
		return queryOrderFcoin(symbol, orderId)
	case model.Coinpark:
		dealAmount, dealPrice, status = queryOrderCoinpark(orderId)
	case model.Coinbig:
		dealAmount, status = queryOrderCoinbig(orderId)
	case model.Bitmex:
		dealAmount, dealPrice, status = queryOrderBitmex(orderId)
	}
	return &model.Order{OrderId: orderId, Symbol: symbol, Market: market, DealAmount: dealAmount, DealPrice: dealPrice,
		Status: status}
}

//func SyncQueryOrderById(market, symbol, orderId string) (order *model.Order) {
//	if orderId == `0` || orderId == `` {
//		return nil
//	}
//	for i := 0; i < 100; i++ {
//		order = QueryOrderById(market, symbol, orderId)
//		if order == nil {
//			continue
//		}
//		if order.Status == model.CarryStatusSuccess || order.Status == model.CarryStatusFail {
//			return order
//		}
//		if i > 10 {
//			cancelResult, cancelErrCode, cancelMsg := CancelOrder(market, symbol, orderId)
//			util.Notice(fmt.Sprintf(`[cancel order] %v %s %s`, cancelResult, cancelErrCode, cancelMsg))
//		}
//		time.Sleep(time.Second * 3)
//	}
//	util.Notice(fmt.Sprintf(`can not query %s %s %s, return %s`, market, symbol, orderId, order.Status))
//	return order
//}

func RefreshCoinAccount(setMarket, symbol, setCoin, accountType string) {
	util.Notice(fmt.Sprintf(`[RefreshCoinAccount]%s %s %s %s`, setMarket, symbol, setCoin, accountType))
	switch setMarket {
	case model.Fcoin:
		if accountType == model.AccountTypeLever {
			setMarket = fmt.Sprintf(`%s_%s_%s`, setMarket, model.AccountTypeLever,
				strings.Replace(symbol, `_`, ``, 1))
			accounts := getLeverAccountFcoin()
			for market, value := range accounts {
				if market == setMarket {
					for coin, account := range value {
						if coin == setCoin {
							util.Notice(fmt.Sprintf(`[update single coin]%s %s %s`, setMarket, symbol, setCoin))
							model.AppAccounts.SetAccount(setMarket, coin, account)
						}
					}
				}
			}
		} else {
			currencies, fcoinAccounts := getAccountFcoin()
			for i := 0; i < len(currencies); i++ {
				if currencies[i] == setCoin {
					util.Notice(fmt.Sprintf(`[update single coin]%s %s %s`, setMarket, symbol, setCoin))
					model.AppAccounts.SetAccount(setMarket, currencies[i], fcoinAccounts[i])
				}
			}
		}
	}
}

//RefreshExclusive
func _(market string, coins map[string]bool) {
	switch market {
	case model.Fcoin:
		accounts := getLeverAccountFcoin()
		for key, value := range accounts {
			for coin, account := range value {
				if coins[coin] == false {
					model.AppAccounts.SetAccount(key, coin, account)
				}
			}
		}
		currencies, fcoinAccounts := getAccountFcoin()
		for i := 0; i < len(currencies); i++ {
			if coins[currencies[i]] == false {
				model.AppAccounts.SetAccount(model.Fcoin, currencies[i], fcoinAccounts[i])
			}
		}
	}
}

func RefreshAccount(market string) {
	util.Notice(`refresh all accounts in market ` + market)
	model.AppAccounts.ClearAccounts(market)
	switch market {
	case model.Huobi:
		getAccountHuobi(model.AppAccounts)
	case model.OKEX:
		getAccountOkex(model.AppAccounts)
	case model.OKFUTURE:
		err := GetAccountOkfuture(model.AppAccounts)
		if err != nil {
			util.Notice(err.Error())
		}
	case model.Binance:
		getAccountBinance(model.AppAccounts)
		//if model.AppConfig.BnbMin > 0 && model.AppConfig.BnbBuy > 0 {
		//	account := model.AppAccounts.GetAccount(model.Binance, `bnb`)
		//	if account != nil && account.Free < model.AppConfig.BnbMin {
		//		util.Notice(fmt.Sprintf(`[bnb數量不足]%f - %f`, account.Free, model.AppConfig.BnbMin))
		//		PlaceOrder(model.OrderSideBuy, model.OrderTypeMarket, model.Binance, `bnb_usdt`, ``,
		//			0, model.AppConfig.BnbBuy)
		//	}
		//}
	case model.Fcoin:
		accounts := getLeverAccountFcoin()
		for key, value := range accounts {
			for coin, account := range value {
				model.AppAccounts.SetAccount(key, coin, account)
			}
		}
		currencies, fcoinAccounts := getAccountFcoin()
		for i := 0; i < len(currencies); i++ {
			model.AppAccounts.SetAccount(model.Fcoin, currencies[i], fcoinAccounts[i])
		}
	case model.Coinpark:
		getAccountCoinpark(model.AppAccounts)
	case model.Coinbig:
		getAccountCoinbig(model.AppAccounts)
	case model.Bitmex:
		getAccountBitmex(model.AppAccounts)
	}
}

// orderSide: OrderSideBuy OrderSideSell OrderSideLiquidateLong OrderSideLiquidateShort
// orderType: OrderTypeLimit OrderTypeMarket
// amount:如果是限价单或市价卖单，amount是左侧币种的数量，如果是市价买单，amount是右测币种的数量
func PlaceOrder(orderSide, orderType, market, symbol, amountType, accountType string, price,
	amount float64) (order *model.Order) {
	if amount == 0 {
		util.Notice(`can not place order with amount 0`)
		return &model.Order{OrderSide: orderSide, OrderType: orderType, Market: market, Symbol: symbol,
			AmountType: amountType, Price: price, Amount: 0, OrderId: ``, ErrCode: ``,
			Status: model.CarryStatusFail, DealAmount: 0, DealPrice: price, OrderTime: util.GetNow()}
	}
	valid := false
	result, bidAsk := model.AppMarkets.GetBidAsk(symbol, market)
	if result {
		if orderSide == model.OrderSideSell && price > bidAsk.Bids[0].Price*0.998 {
			valid = true
		} else if orderSide == model.OrderSideBuy && price < bidAsk.Asks[0].Price*1.002 {
			valid = true
		}
	}
	if !valid {
		util.Notice(fmt.Sprintf(`[place order limit]%s %s %s %f`, market, symbol, orderSide, price))
		return &model.Order{OrderSide: orderSide, OrderType: orderType, Market: market, Symbol: symbol,
			AmountType: amountType, Price: price, Amount: 0, OrderId: ``, ErrCode: ``,
			Status: model.CarryStatusFail, DealAmount: 0, DealPrice: price, OrderTime: util.GetNow()}
	}
	price, strPrice := util.FormatNum(price, GetPriceDecimal(model.Fcoin, symbol))
	amount, strAmount := util.FormatNum(amount, GetAmountDecimal(model.Fcoin, symbol))
	if amountType == model.AmountTypeContractNumber {
		strAmount = strconv.FormatFloat(math.Floor(amount*100)/100, 'f', 2, 64)
	}
	var orderId, errCode, status string
	switch market {
	case model.Huobi:
		orderId, errCode = placeOrderHuobi(orderSide, orderType, symbol, strPrice, strAmount)
	case model.OKEX:
		orderId, errCode = placeOrderOkex(orderSide, orderType, symbol, strPrice, strAmount)
	case model.OKFUTURE:
		if amountType == model.AmountTypeCoinNumber {
			contractAmount := math.Floor(amount * price / model.OKEXOtherContractFaceValue)
			if strings.Contains(symbol, `btc`) {
				contractAmount = math.Floor(amount * price / model.OKEXBTCContractFaceValue)
			}
			if contractAmount < 1 {
				return &model.Order{ErrCode: `amount not enough`, Status: model.CarryStatusFail,
					DealAmount: 0, DealPrice: 0}
			}
			strAmount = strconv.FormatFloat(contractAmount, 'f', 0, 64)
		}
		orderId, errCode = placeOrderOkfuture(orderSide, orderType, symbol, strPrice, strAmount)
	case model.Binance:
		orderId, errCode = placeOrderBinance(orderSide, orderType, symbol, strPrice, strAmount)
	case model.Fcoin:
		orderId, errCode = placeOrderFcoin(orderSide, orderType, symbol, accountType, strPrice, strAmount)
		if orderId == `1002` {
			time.Sleep(time.Millisecond * 200)
		}
	case model.Coinpark:
		orderId, errCode, _ = placeOrderCoinpark(orderSide, orderType, symbol, strPrice, strAmount)
		if errCode == `4003` {
			util.Notice(`【发现4003错误】sleep 3 minutes`)
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
	return &model.Order{OrderSide: orderSide, OrderType: orderType, Market: market, Symbol: symbol,
		AmountType: amountType, Price: price, Amount: amount, OrderId: orderId, ErrCode: errCode,
		Status: status, DealAmount: 0, DealPrice: price, OrderTime: util.GetNow()}
}

func GetPrice(symbol string) (buy float64, err error) {
	if model.AppConfig == nil {
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
	result, price := model.AppMarkets.GetPrice(symbol)
	if result {
		return price, nil
	}
	if model.GetBuyPriceTime[symbol] != 0 && util.GetNowUnixMillion()-model.GetBuyPriceTime[symbol] < 3600000 {
		return model.CurrencyPrice[symbol], nil
	}
	model.GetBuyPriceTime[symbol] = util.GetNowUnixMillion()
	if strs[0] == `BIX` || strs[1] == `BIX` || strs[0] == `CP` || strs[1] == `CP` {
		return getBuyPriceCoinpark(symbol)
	}
	if strs[0] == `FT` || strs[1] == `FT` || model.AppConfig.InChina == 1 {
		return getBuyPriceFcoin(symbol)
	}
	return getBuyPriceFcoin(symbol)
}

//CheckOrderValue
func _(currency string, amount float64) (result bool) {
	currencyPrice, _ := GetPrice(currency + `_usdt`)
	if currencyPrice*amount < model.AppConfig.MinUsdt {
		util.Notice(fmt.Sprintf(`%s下单数量%f不足%f usdt`, currency, amount, model.AppConfig.MinUsdt))
		return false
	}
	return true
}
