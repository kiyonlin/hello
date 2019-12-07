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

func GetMinAmount(market, symbol string) float64 {
	switch market {
	case model.Fcoin:
		switch symbol {
		case `btc_usdt`, `btc_pax`, `btc_tusd`, `btc_usdc`:
			return 0.005
		case `eth_usdt`, `eth_pax`, `eth_usdc`, `eth_btc`, `dash_usdt`, `dash_btc`, `dash_eth`, `bsv_usdt`, `bsv_btc`,
			`bch_usdt`, `bch_btc`:
			return 0.05
		case `ltc_usdt`, `ltc_pax`, `ltc_usdc`, `ltc_btc`, `ltc_eth`, `zec_usdt`, `zec_btc`, `zec_eth`:
			return 0.1
		case `eos_usdt`, `eos_pax`, `eos_usdc`, `eos_btc`, `eos_eth`, `etc_usdt`, `etc_btc`, `etc_eth`, `pax_usdt`,
			`tusd_usdt`, `usdc_usdt`, `gusd_usdt`:
			return 1
		case `xrp_usdt`, `xrp_btc`, `xrp_eth`, `ft_usdt`, `ft_pax`, `fmex_usdt`:
			return 10
		case `iota_usdt`, `iota_btc`, `iota_eth`:
			return 20
		case `xlm_usdt`, `xlm_btc`, `xlm_eth`:
			return 50
		case `ada_usdt`, `ada_btc`, `ada_eth`:
			return 100
		case `trx_usdt`, `trx_btc`, `trx_eth`:
			return 200
		}
	case model.Fmex:
		switch symbol {
		case `btcusd_p`:
			return 1
		}
	case model.Bitmex:
		switch symbol {
		case `btcusd_p`:
			return 1
		case `ethusd_p`:
			return 1
		}

	}
	return 0
}

// 根据不同的网站返回价格小数位
func GetPriceDecimal(market, symbol string) float64 {
	switch market {
	case model.Fcoin:
		//{"status":3022,"msg":"limit price decimal: 5"}
		switch symbol {
		case `btc_usdt`, `bch_usdt`, `btc_pax`, `btc_tusd`, `btc_usdc`, `dash_usdt`:
			return 1
		case `eth_usdt`, `eth_pax`, `eth_usdc`, `ltc_usdt`, `ltc_pax`, `ltc_usdc`, `zec_usdt`, `bsv_usdt`:
			return 2
		case `eos_usdt`, `eos_pax`, `eos_usdc`, `etc_usdt`:
			return 3
		case `ft_usdt`, `xrp_usdt`, `iota_usdt`, `ltc_eth`, `xlm_usdt`, `fmex_usdt`, `pax_usdt`, `tusd_usdt`,
			`usdc_usdt`, `gusd_usdt`, `ft_pax`:
			return 4
		case `eth_btc`, `eos_eth`, `ltc_btc`, `bch_btc`, `etc_eth`, `zec_btc`, `trx_usdt`, `ada_usdt`, `dash_btc`,
			`bsv_btc`:
			return 5
		case `etc_btc`, `xrp_eth`, `iota_eth`, `xlm_eth`, `zec_eth`, `dash_eth`:
			return 6
		case `eos_btc`, `ada_eth`:
			return 7
		case `ft_btc`, `xrp_btc`, `iota_btc`, `ft_eth`, `trx_btc`, `trx_eth`, `xlm_btc`, `ada_btc`:
			return 8
		}
	case model.Fmex:
		switch symbol {
		case `btcusd_p`:
			return 0.5
		}
	case model.Coinpark:
		switch symbol {
		case `cp_usdt`:
			return 4
		case `cp_eth`, `cp_btc`:
			return 8
		}
	case model.Bitmex:
		switch symbol {
		case `btcusd_p`:
			return 0.5
		case `ethusd_p`:
			return 1.5
		}
	}
	return 8
}

func GetAmountDecimal(market, symbol string) float64 {
	switch market {
	case model.OKEX:
		switch symbol {
		case `eos_usdt`, `btc_usdt`:
			return 4
		}
	case model.Fcoin:
		//{"status":3006,"msg":"limit amount decimal: 2"}
		switch symbol {
		case `xrp_btc`, `xrp_eth`, `iota_btc`, `iota_eth`:
			return 0
		case `eos_btc`, `xrp_usdt`, `eos_eth`, `iota_usdt`, `ft_usdt`, `ft_btc`, `ft_eth`, `trx_usdt`, `fmex_usdt`,
			`trx_btc`, `trx_eth`, `xlm_btc`, `ada_btc`, `ft_pax`:
			return 2
		case `btc_usdt`, `btc_pax`, `,btc_tusd`, `btc_usdc`, `eos_usdt`, `eth_btc`, `eth_usdt`, `ltc_usdt`, `ltc_btc`,
			`ltc_eth`, `eth_pax`, `eth_usdc`, `eos_pax`, `eos_usdc`, `ltc_pax`, `ltc_usdc`, `xlm_eth`, `zec_eth`,
			`etc_usdt`, `etc_btc`, `etc_eth`, `bch_btc`, `bch_usdt`, `bsv_usdt`, `zec_usdt`, `xlm_usdt`, `ada_usdt`,
			`ada_eth`, `dash_usdt`, `dash_btc`, `dash_eth`, `bsv_btc`, `pax_usdt`, `tusd_usdt`, `usdc_usdt`, `gusd_usdt`:
			return 4
		}
	case model.Fmex:
		switch symbol {
		case `btcusd_p`:
			return 0
		}
	case model.Bitmex:
		switch symbol {
		case `btcusd_p`:
			return 0
		case `ethusd_p`:
			return 0
		}
	}
	return 4
}

func MustCancel(key, secret, market, symbol, orderId string, mustCancel bool) (res bool, order *model.Order) {
	for i := 0; i < 7; i++ {
		result, errCode, _, cancelOrder := CancelOrder(key, secret, market, symbol, orderId)
		res = result
		order = cancelOrder
		util.Notice(fmt.Sprintf(`[cancel] %s for %d times, return %t `, orderId, i, result))
		if result || !mustCancel || errCode == `0` {
			return result, cancelOrder
		}
		if errCode == `3008` && i >= 3 {
			return result, cancelOrder
		}
		//if result || !mustCancel { //3008:"submit cancel invalid order state
		//	break
		//} else if errCode == `429` || errCode == `4003` {
		//	util.Notice(`调用次数繁忙`)
		//}
		if i < 2 {
			time.Sleep(time.Second)
		} else if i >= 2 && i < 5 {
			time.Sleep(time.Second * 3)
		} else if i == 5 {
			time.Sleep(time.Second * 10)
		}
	}
	return res, order
}

func CancelOrder(key, secret, market string, symbol string, orderId string) (
	result bool, errCode, msg string, order *model.Order) {
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
		result, errCode, msg = CancelOrderFcoin(key, secret, orderId)
	case model.Coinpark:
		result, errCode, msg = CancelOrderCoinpark(orderId)
	case model.Bitmex:
		result, errCode, msg = CancelOrderBitmex(key, secret, orderId)
	case model.Fmex:
		result, errCode, msg, order = CancelOrderFmex(key, secret, orderId)
		if order != nil {
			order.Symbol = symbol
		}
	}
	util.Notice(fmt.Sprintf(`[cancel %s %v %s %s]`, orderId, result, market, symbol))
	return result, errCode, msg, order
}

func QueryOrders(key, secret, market, symbol, states, accountTypes string, before, after int64) (orders []*model.Order) {
	switch market {
	case model.Fcoin:
		orders = make([]*model.Order, 0)
		if strings.Contains(accountTypes, model.AccountTypeNormal) {
			normal := queryOrdersFcoin(key, secret, symbol, states, model.AccountTypeNormal, before, after)
			for _, value := range normal {
				orders = append(orders, value)
			}
		}
		if strings.Contains(accountTypes, model.AccountTypeLever) {
			lever := queryOrdersFcoin(key, secret, symbol, states, model.AccountTypeLever, before, after)
			for _, value := range lever {
				orders = append(orders, value)
			}
		}
		return orders
	case model.Fmex:
		return queryOrdersFmex(key, secret, symbol)
	default:
		util.Notice(market + ` not supported`)
	}
	return nil
}

func GetDayCandle(key, secret, market, symbol string, timeCandle time.Time) (candle *model.Candle) {
	candle = model.GetCandle(market, symbol, `1d`, timeCandle)
	if candle != nil && candle.N > 0 {
		return
	}
	candle = &model.Candle{}
	model.AppDB.Where(`market = ? and symbol = ? and period = ? and start = ?`,
		market, symbol, `1d`, timeCandle.String()[0:19]).First(candle)
	if candle.N > 0 {
		return
	}
	dBegin, _ := time.ParseDuration(`-456h`)
	dEnd, _ := time.ParseDuration(`24h`)
	begin := timeCandle.Add(dBegin)
	end := timeCandle.Add(dEnd)
	//fmt.Println(begin.String() + `api========>` + end.String())
	switch market {
	case model.Bitmex:
		candles := getCandlesBitmex(key, secret, symbol, `1d`, begin, end, 20)
		for _, value := range candles {
			c := model.GetCandle(value.Market, value.Symbol, value.Period, value.Start)
			if c == nil || c.N == 0 {
				candleDB := &model.Candle{}
				model.AppDB.Where(`market = ? and symbol = ? and period = ? and start = ?`,
					market, symbol, `1d`, value.Start.String()[0:10]).First(candleDB)
				candleDB.Start = time.Date(candleDB.Start.Year(), candleDB.Start.Month(), candleDB.Start.Day(),
					0, 0, 0, 0, util.GetNow().Location())
				if candleDB.N > 0 {
					value.N = candleDB.N
				}
				model.SetCandle(market, symbol, `1d`, value.Start, value)
			}
		}
	}
	candle = model.GetCandle(market, symbol, `1d`, timeCandle)
	if candle == nil {
		util.Notice(fmt.Sprintf(`error: can not get candle %s %s`, `1d`, timeCandle.String()))
		return
	}
	candle.N = (candle.PriceHigh - candle.PriceLow) / 20
	for i := 1; i < 20; i++ {
		d, _ := time.ParseDuration(fmt.Sprintf(`%dh`, -24*i))
		index := timeCandle.Add(d)
		candleCurrent := model.GetCandle(market, symbol, `1d`, index)
		if candleCurrent == nil {
			util.Notice(fmt.Sprintf(`error: can not get candle %s %s`, `1d`, index.String()))
			continue
		}
		if candleCurrent.N > 0 {
			candle.N += candleCurrent.N / 20
		} else {
			candle.N = candle.N + (candleCurrent.PriceHigh-candleCurrent.PriceLow)/20
		}
	}
	fmt.Println(candle.Start.String())
	model.AppDB.Save(&candle)
	model.SetCandle(market, symbol, `1d`, timeCandle, candle)
	return candle
}

func GetBtcBalance(key, secret, market string) (balance float64) {
	today := util.GetNow()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	balance = model.GetBtcBalance(market, today)
	if balance > 0 {
		return
	}
	switch market {
	case model.Bitmex:
		balance = getBtcBalanceBitmex(key, secret)
		model.SetBtcBalance(market, today, balance)
	}
	return
}

func GetFundingRate(market, symbol string) (fundingRate float64) {
	rate, expireTime := model.GetFundingRate(market, symbol)
	now := util.GetNow().Unix()
	if now-60 < expireTime {
		return rate
	}
	util.Notice(fmt.Sprintf(`before update funding %s %s rate %f expire %d`, market, symbol, rate, expireTime))
	switch market {
	case model.Fmex:
		fundingRate, expireTime = getFundingRateFmex(symbol)
	case model.Bitmex:
		fundingRate, expireTime = getFundingRateBitmex(symbol)
	}
	model.SetFundingRate(market, symbol, fundingRate, expireTime)
	util.Notice(fmt.Sprintf(`after update funding %s %s rate %f expire %d`,
		market, symbol, fundingRate, expireTime))
	return fundingRate
}

func QueryOrderById(key, secret, market, symbol, orderId string) (order *model.Order) {
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
		return queryOrderFcoin(key, secret, symbol, orderId)
	case model.Coinpark:
		dealAmount, dealPrice, status = queryOrderCoinpark(orderId)
	case model.Bitmex:
		orders := queryOrderBitmex(key, secret, symbol, orderId)
		for _, value := range orders {
			if value.OrderId == orderId {
				return value
			}
		}
		return nil
	case model.Fmex:
		order = queryOrderFmex(key, secret, orderId)
		return order
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

func RefreshCoinAccount(key, secret, setMarket, symbol, setCoin, accountType string) {
	util.Notice(fmt.Sprintf(`[RefreshCoinAccount]%s %s %s %s`, setMarket, symbol, setCoin, accountType))
	switch setMarket {
	case model.Fcoin:
		if accountType == model.AccountTypeLever {
			setMarket = fmt.Sprintf(`%s_%s_%s`, setMarket, model.AccountTypeLever,
				strings.Replace(symbol, `_`, ``, 1))
			accounts := getLeverAccountFcoin(key, secret)
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
		} else if accountType == model.AccountTypeNormal {
			currencies, fcoinAccounts := getAccountFcoin(key, secret)
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
func _(key, secret, market string, coins map[string]bool) {
	switch market {
	case model.Fcoin:
		accounts := getLeverAccountFcoin(key, secret)
		for key, value := range accounts {
			for coin, account := range value {
				if coins[coin] == false {
					model.AppAccounts.SetAccount(key, coin, account)
				}
			}
		}
		currencies, fcoinAccounts := getAccountFcoin(key, secret)
		for i := 0; i < len(currencies); i++ {
			if coins[currencies[i]] == false {
				model.AppAccounts.SetAccount(model.Fcoin, currencies[i], fcoinAccounts[i])
			}
		}
	}
}

func RefreshAccount(key, secret, market string) {
	util.SocketInfo(`refresh all accounts in market ` + market)
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
		accounts := getLeverAccountFcoin(key, secret)
		for key, value := range accounts {
			for coin, account := range value {
				model.AppAccounts.SetAccount(key, coin, account)
			}
		}
		currencies, fcoinAccounts := getAccountFcoin(key, secret)
		for i := 0; i < len(currencies); i++ {
			model.AppAccounts.SetAccount(model.Fcoin, currencies[i], fcoinAccounts[i])
		}
	case model.Fmex:
		accounts := getAccountFmex(key, secret)
		for _, account := range accounts {
			model.AppAccounts.SetAccount(model.Fmex, account.Currency, account)
		}
	case model.Coinpark:
		getAccountCoinpark(model.AppAccounts)
	case model.Bitmex:
		getAccountBitmex(key, secret, model.AppAccounts)
	}
}

// orderSide: OrderSideBuy OrderSideSell OrderSideLiquidateLong OrderSideLiquidateShort
// orderType: OrderTypeLimit OrderTypeMarket
// amount:如果是限价单或市价卖单，amount是左侧币种的数量，如果是市价买单，amount是右测币种的数量
func PlaceOrder(key, secret, orderSide, orderType, market, symbol, amountType, accountType, orderParam string,
	price, amount float64, saveDB bool) (order *model.Order) {
	start := util.GetNowUnixMillion()
	util.Notice(fmt.Sprintf(`...%s %s %s before order %d amount:%f price:%f`,
		orderSide, market, symbol, start, amount, price))
	if amount < 0.0001 {
		util.Notice(`can not place order with amount 0`)
		return &model.Order{OrderSide: orderSide, OrderType: orderType, Market: market, Symbol: symbol,
			AmountType: amountType, Price: price, Amount: 0, OrderId: ``, ErrCode: ``,
			Status: model.CarryStatusFail, DealAmount: 0, DealPrice: price, OrderTime: util.GetNow()}
	}
	//valid := false
	//result, bidAsk := model.AppMarkets.GetBidAsk(symbol, market)
	//if result {
	//	if orderSide == model.OrderSideSell && price > bidAsk.Bids[0].Price*0.998 {
	//		valid = true
	//	} else if orderSide == model.OrderSideBuy && price < bidAsk.Asks[0].Price*1.002 {
	//		valid = true
	//	}
	//}
	//if !valid {
	//	util.Notice(fmt.Sprintf(`[place order limit]%s %s %s %f`, market, symbol, orderSide, price))
	//	return &model.Order{OrderSide: orderSide, OrderType: orderType, Market: market, Symbol: symbol,
	//		AmountType: amountType, Price: price, Amount: 0, OrderId: ``, ErrCode: ``,
	//		Status: model.CarryStatusFail, DealAmount: 0, DealPrice: price, OrderTime: util.GetNow()}
	//}
	price, strPrice := util.FormatNum(price, GetPriceDecimal(market, symbol))
	amount, strAmount := util.FormatNum(amount, GetAmountDecimal(market, symbol))
	if amountType == model.AmountTypeContractNumber {
		strAmount = strconv.FormatFloat(math.Floor(amount*100)/100, 'f', 2, 64)
	}
	order = &model.Order{OrderSide: orderSide, OrderType: orderType, Market: market, Symbol: symbol,
		AmountType: amountType, Price: price, Amount: amount, DealAmount: 0, DealPrice: price,
		OrderTime: util.GetNow()}
	switch market {
	case model.Huobi:
		placeOrderHuobi(order, orderSide, orderType, symbol, strPrice, strAmount)
	case model.OKEX:
		placeOrderOkex(order, orderSide, orderType, symbol, strPrice, strAmount)
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
		placeOrderOkfuture(order, orderSide, orderType, symbol, strPrice, strAmount)
	case model.Binance:
		placeOrderBinance(order, orderSide, orderType, symbol, strPrice, strAmount)
	case model.Fcoin:
		placeOrderFcoin(order, key, secret, orderSide, orderType, symbol, accountType, strPrice, strAmount)
		if order.ErrCode == `1002` {
			time.Sleep(time.Millisecond * 200)
		}
	case model.Fmex:
		placeOrderFmex(order, key, secret, orderSide, orderType, symbol, strPrice, strAmount)
	case model.Coinpark:
		placeOrderCoinpark(order, orderSide, orderType, symbol, strPrice, strAmount)
		if order.ErrCode == `4003` {
			util.Notice(`【发现4003错误】sleep 3 minutes`)
			time.Sleep(time.Minute * 3)
		}
	case model.Bitmex:
		placeOrderBitmex(order, key, secret, orderSide, orderType, orderParam, symbol, strPrice, strAmount)
	}
	if order.OrderId == "0" || order.OrderId == "" {
		order.Status = model.CarryStatusFail
	} else if order.Status == `` {
		order.Status = model.CarryStatusWorking
	}
	end := util.GetNowUnixMillion()
	util.Notice(fmt.Sprintf(`...%s %s %s return order %d distance %d`,
		orderSide, market, symbol, end, end-start))
	if saveDB {
		go model.AppDB.Save(&order)
	}
	return
}

func GetPrice(key, secret, symbol string) (buy float64, err error) {
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
	price = model.AppConfig.SymbolPrice[symbol]
	if price > 0 && util.GetNowUnixMillion()-model.AppConfig.UpdatePriceTime[symbol] < 3600000 {
		return price, nil
	}
	model.AppConfig.SetUpdatePriceTime(symbol, util.GetNowUnixMillion())
	if strs[0] == `BIX` || strs[1] == `BIX` || strs[0] == `CP` || strs[1] == `CP` {
		return getBuyPriceCoinpark(symbol)
	}
	if strs[0] == `FT` || strs[1] == `FT` || model.AppConfig.InChina == 1 {
		return getBuyPriceFcoin(key, secret, symbol)
	}
	return getBuyPriceFcoin(key, secret, symbol)
}

//CheckOrderValue
func _(key, secret, currency string, amount float64) (result bool) {
	currencyPrice, _ := GetPrice(key, secret, currency+`_usdt`)
	if currencyPrice*amount < model.AppConfig.MinUsdt {
		util.Notice(fmt.Sprintf(`%s下单数量%f不足%f usdt`, currency, amount, model.AppConfig.MinUsdt))
		return false
	}
	return true
}
