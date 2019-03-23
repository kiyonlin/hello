package model

import (
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	"strings"
	"sync"
)

var HandlerMap = make(map[string]CarryHandler)

const ArbitraryCarryUSDT = 100.0
const OKEXBTCContractFaceValue = 100.0
const OKEXOtherContractFaceValue = 10.0
const OKEX = "okex"
const OKFUTURE = `okfuture`
const Huobi = "huobi"
const Binance = "binance"
const Fcoin = "fcoin"
const Coinbig = "coinbig"
const Coinpark = "coinpark"
const Btcdo = `btcdo`
const Bitmex = `bitmex`

const SubscribeDepth = `SubscribeDepth`
const SubscribeDeal = `subscribeDeal`
const CarryStatusSuccess = "success"
const CarryStatusFail = "fail"
const CarryStatusWorking = "working"
const OrderTypeLimit = `limit`
const OrderTypeMarket = `market`
const OrderSideBuy = `buy`
const OrderSideSell = `sell`
const OrderSideLiquidateLong = `liquidateLong`
const OrderSideLiquidateShort = `liquidateShort`
const CarryTypeBalance = `balance`
const CarryTypeFuture = `future`
const CarryTypeArbitrarySell = `arbitrarysell`
const CarryTypeArbitraryBuy = `arbitrarybuy`
const AmountTypeContractNumber = `contractnumber`
const AmountTypeCoinNumber = `coinnumber`

const FunctionGrid = `grid`
const FunctionMaker = `maker`
const FunctionCarry = `carry`
const FunctionBalanceTurtle = `balanceturtle`
const FunctionArbitrary = `arbitrary`
const FunctionRefresh = `refresh`

const FunRefreshMiddle = `refresh_parameter_middle`
const FunRefreshSide = `refresh_parameter_side`

var AppDB *gorm.DB
var AppSettings []Setting
var AppConfig *Config
var AppMarkets = NewMarkets()
var AppAccounts = NewAccounts()

var HuobiAccountId = ""
var CarryChannel = make(chan Carry, 50)
var AccountChannel = make(chan map[string]*Account, 50)
var InnerCarryChannel = make(chan Carry, 50)
var CurrencyPrice = make(map[string]float64)
var GetBuyPriceTime = make(map[string]int64)
var BalanceTurtleCarries = make(map[string]map[string]*Carry) // market - symbol - *carry

var dictMap = map[string]map[string]string{ // market - union name - market name
	Fcoin: {
		OrderTypeLimit:  `limit`,
		OrderTypeMarket: `market`,
		OrderSideBuy:    `buy`,
		OrderSideSell:   `sell`,
	},
}

var orderStatusMap = map[string]map[string]string{ // market - market status - united status
	Binance: {
		"NEW":              CarryStatusWorking,
		"PARTIALLY_FILLED": CarryStatusWorking,
		"PENDING_CANCEL":   CarryStatusSuccess,
		"FILLED":           CarryStatusSuccess,
		"CANCELED":         CarryStatusFail,
		"REJECTED":         CarryStatusFail,
		"EXPIRED":          CarryStatusFail},
	Huobi: {
		`submitting`:       CarryStatusWorking, //已提交
		`submitted`:        CarryStatusWorking, //已提交,
		`partial-filled`:   CarryStatusWorking, //部分成交,
		`partial-canceled`: CarryStatusSuccess, //部分成交撤销,
		`filled`:           CarryStatusSuccess, //完全成交,
		`canceled`:         CarryStatusFail},   //已撤销
	OKEX: {
		"-1": CarryStatusFail,    //已撤销
		"0":  CarryStatusWorking, //未成交
		"1":  CarryStatusWorking, //部分成交
		"2":  CarryStatusSuccess, //完全成交
		"3":  CarryStatusWorking, //撤单处理中
	},
	OKFUTURE: {
		`0`:  CarryStatusWorking, //等待成交
		`1`:  CarryStatusWorking, //部分成交
		`2`:  CarryStatusSuccess, //全部成交
		`-1`: CarryStatusFail,    //撤单
		`4`:  CarryStatusWorking, //撤单处理中
		`5`:  CarryStatusWorking, //撤单中)
	},
	Fcoin: {
		`submitted`:        CarryStatusWorking, //已提交
		`partial_filled`:   CarryStatusWorking, //部分成交
		`partial_canceled`: CarryStatusSuccess, //部分成交已撤销
		`filled`:           CarryStatusSuccess, //完全成交
		`canceled`:         CarryStatusFail,    //已撤销
		`pending_cancel`:   CarryStatusFail,    //撤销已提交
	},
	Coinpark: {
		`1`: CarryStatusWorking, //待成交
		`2`: CarryStatusSuccess, //部分成交
		`3`: CarryStatusSuccess, //完全成交
		`4`: CarryStatusSuccess, //部分撤销
		`5`: CarryStatusFail,    //完全撤销
		`6`: CarryStatusWorking, //待撤销
	},
	Coinbig: {
		`1`: CarryStatusWorking, //未成交
		`2`: CarryStatusWorking, //部分成交,
		`3`: CarryStatusSuccess, //完全成交,
		`4`: CarryStatusFail,    //用户撤销,
		`5`: CarryStatusSuccess, //部分撤回,
		`6`: CarryStatusFail,    //成交失败
	},
}

func GetOrderStatusRevert(market, status string) (combinedStatus string, err error) {
	combinedStatus = ``
	if orderStatusMap[market] == nil {
		return ``, errors.New(`no market ` + market)
	}
	for key, value := range orderStatusMap[market] {
		if value == status {
			if combinedStatus != `` {
				combinedStatus += `,`
			}
			combinedStatus += key
		}
	}
	return combinedStatus, nil
}

func GetDictMapRevert(market, marketWord string) (uninoWord string) {
	if dictMap[market] == nil {
		return ``
	}
	for key, value := range dictMap[market] {
		if value == marketWord {
			return key
		}
	}
	return ``
}

func GetDictMap(market, unionWord string) (marketWord string) {
	if dictMap[market] == nil {
		return ``
	}
	return dictMap[market][unionWord]
}

func GetOrderStatus(market, marketStatus string) (status string) {
	if orderStatusMap[market] == nil {
		return CarryStatusWorking
	}
	if orderStatusMap[market][marketStatus] == `` {
		return CarryStatusWorking
	}
	return orderStatusMap[market][marketStatus]
}

func GetBalanceTurtleCarry(market, symbol string) (turtleCarry *Carry) {
	if BalanceTurtleCarries[market] == nil {
		return nil
	}
	return BalanceTurtleCarries[market][symbol]
}

func SetBalanceTurtleCarry(market, symbol string, turtleCarry *Carry) {
	if BalanceTurtleCarries[market] == nil {
		BalanceTurtleCarries[market] = make(map[string]*Carry)
	}
	BalanceTurtleCarries[market][symbol] = turtleCarry
}

func getWSSubscribes(market, symbol, subType string) (subscribe string) {
	switch market {
	case Huobi: // xrp_btc: market.xrpbtc.depth.step0
		return "market." + strings.Replace(symbol, "_", "", 1) + ".depth.step0"
	case OKEX: // xrp_btc: ok_sub_spot_xrp_btc_depth_5
		return "ok_sub_spot_" + symbol + "_depth_5"
	case OKFUTURE:
		index := strings.Index(symbol, `_`)
		if index != -1 {
			// btc_this_week: ok_sub_futureusd_btc_depth_this_week
			//return `ok_sub_futureusd_` + symbol[0:index] + `_depth` + symbol[index:]
			// btc_this_week: ok_sub_futureusd_btc_ticker_this_week
			//return `ok_sub_futureusd_` + symbol[0:index] + `_ticker` + symbol[index:]
			// btc_this_week: ok_sub_futureusd_X_depth_Y_Z
			return `ok_sub_futureusd_` + symbol[0:index] + `_depth` + symbol[index:] + `_5`
		}
		return
	case Binance: // xrp_btc: xrpbtc@depth5
		return strings.ToLower(strings.Replace(symbol, "_", "", 1)) + `@depth5`
	case Fcoin:
		if subType == SubscribeDeal {
			// btc_usdt: trade.btcusdt
			return `trade.` + strings.ToLower(strings.Replace(symbol, "_", "", 1))
		} else {
			// btc_usdt: depth.L20.btcusdt
			return `depth.L20.` + strings.ToLower(strings.Replace(symbol, "_", "", 1))
		}
	case Coinpark: //BTC_USDT bibox_sub_spot_BTC_USDT_ticker
		//return `bibox_sub_spot_` + strings.ToUpper(symbol) + `_ticker`
		return `bibox_sub_spot_` + strings.ToUpper(symbol) + `_depth`
	case Btcdo:
		return strings.ToUpper(symbol)
	case Bitmex:
		return symbol
	case Coinbig:
		switch symbol {
		case `btc_usdt`:
			return `27`
		case `eth_usdt`:
			return `28`
		}
	}
	return ""
}

func getSymbolWithSplit(original, split string) (symbol string) {
	original = strings.ToLower(original)
	var moneyCurrency string
	if strings.Contains(original, `btc`) && strings.LastIndex(original, "btc")+3 == len(original) {
		moneyCurrency = "btc"
	} else if strings.Contains(original, `usdt`) && strings.LastIndex(original, "usdt")+4 == len(original) {
		moneyCurrency = "usdt"
	} else if strings.Contains(original, `eth`) && strings.LastIndex(original, "eth")+3 == len(original) {
		moneyCurrency = "eth"
	} else if strings.Contains(original, `ft1808`) && strings.LastIndex(original, `ft1808`)+6 == len(original) {
		moneyCurrency = `ft1808`
	}
	return original[0:strings.LastIndex(original, moneyCurrency)] + split + moneyCurrency
}

func GetSymbol(market, subscribe string) (symbol string) {
	switch market {
	case Huobi: //market.xrpbtc.depth.step0: xrp_btc
		subscribe = strings.Replace(subscribe, "market.", "", 1)
		subscribe = strings.Replace(subscribe, ".depth.step0", "", 1)
		return getSymbolWithSplit(subscribe, "_")
	case OKEX: //ok_sub_spot_xrp_btc_depth_5: xrp_btc
		subscribe = strings.Replace(subscribe, "ok_sub_spot_", "", 1)
		subscribe = strings.Replace(subscribe, "_depth_5", "", 1)
		return subscribe
	case OKFUTURE: // ok_sub_futureusd_btc_depth_this_week_5: btc_this_week
		subscribe = strings.Replace(subscribe, `ok_sub_futureusd_`, ``, 1)
		subscribe = strings.Replace(subscribe, `_depth`, ``, 1)
		subscribe = strings.Replace(subscribe, `_5`, ``, 1)
		return subscribe
	case Binance: // eosusdt@depth5: xrp_btc
		if strings.Index(subscribe, `@`) == -1 {
			return ``
		}
		subscribe = subscribe[0:strings.Index(subscribe, `@`)]
		return getSymbolWithSplit(subscribe, `_`)
	case Fcoin: // btc_usdt: depth.L20.btcusdt
		subscribe = strings.Replace(subscribe, "depth.L20.", "", 1)
		return getSymbolWithSplit(subscribe, "_")
	case Coinpark: //BTC_USDT bibox_sub_spot_BTC_USDT_ticker
		subscribe = strings.Replace(subscribe, `bibox_sub_spot_`, ``, 1)
		subscribe = strings.Replace(subscribe, `_ticker`, ``, 1)
		return subscribe
	case Coinbig: // btc_usdt 27 // eth_usdt 28
		switch subscribe {
		case `27`:
			return `btc_usdt`
		case `28`:
			return `eth_usdt`
		}
	case Bitmex:
		return subscribe
	}
	return ""
}

type Config struct {
	lock              sync.Mutex
	Env               string
	DBConnection      string
	Channels          int
	ChannelSlot       float64
	Delay             float64
	Deduction         float64
	MinUsdt           float64            // 折合usdt最小下单金额
	MaxUsdt           float64            // 折合usdt最大下单金额
	WSUrls            map[string]string  // marketName - ws url
	RestUrls          map[string]string  // marketName - rest url
	MarketCost        map[string]float64 // marketName - order cost
	HuobiKey          string
	HuobiSecret       string
	OkexKey           string
	OkexSecret        string
	BinanceKey        string
	BinanceSecret     string
	CoinbigKey        string
	CoinbigSecret     string
	CoinparkKey       string
	CoinparkSecret    string
	BitmexKey         string
	BitmexSecret      string
	FcoinKey          string
	FcoinSecret       string
	BnbMin            float64
	BnbBuy            float64
	OrderWait         int64 // fcoin/coinpark 刷单平均等待时间
	WaitRefreshRandom int64
	WaitMaker         int64
	AmountRate        float64 // 刷单填写数量比率
	MakerAmountRate   float64
	Handle            string // 0 不执行处理程序，1执行处理程序
	HandleMaker       string
	HandleRefresh     string
	HandleGrid        string
	SellRate          float64 // fcoin dk 额外卖单下单比例
	FtMax             float64 // fcoin dk ft上限
	InChina           int     // 1 in china, otherwise outter china
	Mail              string
	Port              string
}

func NewConfig() {
	AppConfig = &Config{}
	AppConfig.WSUrls = make(map[string]string)
	//AppConfig.WSUrls[Huobi] = "wss://api.huobi.pro/ws"
	AppConfig.WSUrls[Huobi] = `wss://api.huobi.br.com/ws`
	AppConfig.WSUrls[OKEX] = "wss://real.okex.com:10441/websocket?compress=true"
	AppConfig.WSUrls[OKFUTURE] = `wss://real.okex.com:10440/websocket?compress=true`
	AppConfig.WSUrls[Binance] = "wss://stream.binance.com:9443/stream?streams="
	AppConfig.WSUrls[Fcoin] = "wss://api.fcoin.com/v2/ws"
	AppConfig.WSUrls[Coinbig] = "wss://ws.coinbig.com/ws"
	AppConfig.WSUrls[Coinpark] = "wss://push.coinpark.cc/"
	AppConfig.WSUrls[Btcdo] = `wss://onli-quotation.btcdo.com/v1/market/?EIO=3&transport=websocket`
	//AppConfig.WSUrls[Bitmex] = `wss://www.bitmex.com/realtime/`
	AppConfig.WSUrls[Bitmex] = `wss://testnet.bitmex.com/realtime`
	AppConfig.RestUrls = make(map[string]string)
	// HUOBI用于交易的API，可能不适用于行情
	//config.RestUrls[Huobi] = "https://api.huobipro.com/v1"
	//AppConfig.RestUrls[Huobi] = "https://api.huobi.pro"
	AppConfig.RestUrls[Fcoin] = "https://api.fcoin.com/v2"
	AppConfig.RestUrls[Huobi] = `https://api.huobi.br.com`
	AppConfig.RestUrls[OKEX] = "https://www.okex.com/api/v1"
	AppConfig.RestUrls[OKFUTURE] = `https://www.okex.com/api/v1`
	AppConfig.RestUrls[Binance] = "https://api.binance.com"
	AppConfig.RestUrls[Coinbig] = "https://www.coinbig.com/api/publics/v1"
	AppConfig.RestUrls[Coinpark] = "https://api.coinpark.cc/v1"
	AppConfig.RestUrls[Btcdo] = `https://api.btcdo.com`
	//AppConfig.RestUrls[Bitmex] = `https://www.bitmex.com/api/v1`
	AppConfig.RestUrls[Bitmex] = `https://testnet.bitmex.com/api/v1`
	AppConfig.MarketCost = make(map[string]float64)
	AppConfig.MarketCost[Fcoin] = 0
	AppConfig.MarketCost[Huobi] = 0.0005
	AppConfig.MarketCost[OKEX] = 0.0005
	AppConfig.MarketCost[OKFUTURE] = 0.0005
	AppConfig.MarketCost[Binance] = 0.0004
	AppConfig.MarketCost[Coinpark] = 0
	AppConfig.MarketCost[Coinbig] = 0
	AppConfig.MarketCost[Bitmex] = 0.0005
}

func GetAccountInfoSubscribe(marketName string) []string {
	switch marketName {
	case OKFUTURE:
		//return []string{`ok_sub_futureusd_userinfo`}
		return []string{`login`}
	}
	return nil
}

func (config *Config) ToString() string {
	str := "markets-carry cost:\n"
	for key, value := range config.MarketCost {
		str += fmt.Sprintf("-%s base carry cost: %f\n", key, value)
	}
	str += fmt.Sprintf("deduction: %f\n", config.Deduction)
	str += fmt.Sprintf("delay: %f\n", config.Delay)
	str += fmt.Sprintf("channelslot: %f\n", config.ChannelSlot)
	str += fmt.Sprintf("minusdt: %f\n", config.MinUsdt)
	str += fmt.Sprintf("maxusdt: %f\n", config.MaxUsdt)
	str += "env: " + config.Env + "\n"
	str += fmt.Sprintf("channels: %d\n", config.Channels)
	str += fmt.Sprintf("handle: %s handleMaker: %s handlerefresh: %s handlegrid: %s\n",
		config.Handle, config.HandleMaker, config.HandleRefresh, config.HandleGrid)
	str += fmt.Sprintf("orderwait: %d amountrate: %f sellrate %f ftmax %f waitMaker: %d\n",
		config.OrderWait, config.AmountRate, config.SellRate, config.FtMax, config.WaitMaker)
	str += fmt.Sprintf("maker rate: %f waitrefreshrandom: %d\n", config.MakerAmountRate, config.WaitRefreshRandom)
	return str
}

func GetWSSubscribes(market, subType string) []string {
	settings := GetMarketSettings(market)
	subscribes := make([]string, len(settings))
	i := 0
	for symbol := range settings {
		subscribes[i] = getWSSubscribes(market, symbol, subType)
		i++
	}
	return subscribes
}
