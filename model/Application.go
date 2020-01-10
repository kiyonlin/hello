package model

import (
	"fmt"
	"github.com/jinzhu/configor"
	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	"hello/util"
	"strings"
	"sync"
	"time"
)

var HandlerMap = make(map[string]CarryHandler)
var infoLock sync.Mutex
var fundingRate = make(map[string]map[string]float64)     // market - symbol - funding rate
var fundingRateUpdate = make(map[string]map[string]int64) // market - symbol - update time
var Currencies = []string{`btc`, `eth`, `usdt`, `ft`, `ft1808`, `pax`, `usdc`, `tusd`}
var ConnectionResetTime = int64(0)
var btcBalance = make(map[string]float64) // market+rfc3339, btc balance
var candles = make(map[string]*Candle)    // market+symbol+period+rfc3339, candle
var CarryInfo = make(map[string]string)   // function - msg

//const ArbitraryCarryUSDT = 100.0
const OKEXBTCContractFaceValue = 100.0
const OKEXOtherContractFaceValue = 10.0
const Bybit = `bybit`
const OKEX = "okex"
const OKFUTURE = `okfuture`
const Huobi = "huobi"
const Binance = "binance"
const Fcoin = "fcoin"
const Fmex = `fmex`
const Coinbig = "coinbig"
const Coinpark = "coinpark"
const Btcdo = `btcdo`
const Bitmex = `bitmex`
const AccountTypeNormal = ``
const AccountTypeLever = `lever`
const SubscribeDepth = `SubscribeDepth`
const SubscribeDeal = `subscribeDeal`
const CarryStatusSuccess = "success"
const CarryStatusFail = "fail"
const CarryStatusWorking = "working"
const OrderTypeLimit = `limit`
const OrderTypeMarket = `market`
const OrderTypeStop = `stop`
const OrderSideBuy = `buy`
const OrderSideSell = `sell`
const OrderSideLiquidateLong = `liquidateLong`
const OrderSideLiquidateShort = `liquidateShort`

//const CarryTypeFuture = `future`
//const CarryTypeArbitrarySell = `arbitrarysell`
//const CarryTypeArbitraryBuy = `arbitrarybuy`
const AmountTypeContractNumber = `contractnumber`
const AmountTypeCoinNumber = `coinnumber`

const FunctionTurtle = `turtle`
const FunctionGrid = `grid`
const FunctionMaker = `maker`
const FunctionCarry = `carry`
const FunctionHang = `hang`
const FunctionHangFar = `hang_far`
const FunctionHangContract = `hang_contract`
const FunctionRank = `rank`
const FunctionHangRevert = `hang_revert`
const FunctionBMCarryHang = `bm_carry_hang`

//const FunctionArbitrary = `arbitrary`
const FunctionRefresh = `refresh`

//const FunRefreshMiddle = `refresh_parameter_middle`
//const FunRefreshSide = `refresh_parameter_side`
//const FunRefreshSeparate = `refresh_parameter_separate`

var AppDB *gorm.DB
var AppSettings []Setting
var AppConfig *Config
var AppMarkets = NewMarkets()
var AppAccounts = NewAccounts()
var HuobiAccountId = ""
var AppPause = false

//var CarryChannel = make(chan Carry, 50)
//var InnerCarryChannel = make(chan Carry, 50)
var AccountChannel = make(chan map[string]*Account, 50)

type Config struct {
	lock            sync.Mutex
	Env             string
	DBConnection    string
	Channels        int
	InChina         int // 1 in china, otherwise outter china
	RefreshTimeSlot int
	//SequencePlace   int // position:[0~14]
	Between         int64
	OrderWait       int64 // fcoin/coinpark 刷单平均等待时间
	ChannelSlot     float64
	Delay           float64
	MinUsdt         float64 // 折合usdt最小下单金额
	MaxUsdt         float64 // 折合usdt最大下单金额
	Stable          bool
	WSUrls          map[string]string  // marketName - ws url
	RestUrls        map[string]string  // marketName - rest url
	MarketCost      map[string]float64 // marketName - order cost
	HuobiKey        string
	HuobiSecret     string
	OkexKey         string
	OkexSecret      string
	BinanceKey      string
	BinanceSecret   string
	CoinbigKey      string
	CoinbigSecret   string
	CoinparkKey     string
	CoinparkSecret  string
	BitmexKey       string
	BitmexSecret    string
	BybitKey        string
	BybitSecret     string
	FcoinKey        string
	FcoinSecret     string
	CarryDistance   float64 // carry价差触发条件
	AmountRate      float64 // 刷单填写数量比率
	PreDealDis      float64
	BinanceOrderDis float64
	Handle          string // 0 不执行处理程序，1执行处理程序
	HandleMaker     string
	HandleRefresh   string
	HandleGrid      string
	Mail            string
	Port            string
	TurtleLimitMain int
	SymbolPrice     map[string]float64 // symbol - price
	UpdatePriceTime map[string]int64   // symbol -time
}

var DialectSymbol = map[string]map[string]string{ // market - standard symbol - dialect symbol
	Bitmex: {
		`btcusd_p`: `XBTUSD`,
		`ethusd_p`: `ETHUSD`,
	},
	Bybit: {
		`btcusd_p`: `BTCUSD`,
		`ethusd_p`: `ETHUSD`,
	},
}

var StandardSymbol = map[string]map[string]string{ // market - dialect symbol - standard symbol
	Bitmex: {
		`XBTUSD`: `btcusd_p`,
		`ETHUSD`: `ethusd_p`,
	},
	Bybit: {
		`BTCUSD`: `btcusd_p`,
		`ETHUSD`: `ethusd_p`,
	},
}
var dictMap = map[string]map[string]string{ // market - union name - market name
	Fcoin: {
		OrderTypeLimit:  `limit`,
		OrderTypeMarket: `market`,
		OrderSideBuy:    `buy`,
		OrderSideSell:   `sell`,
	},
	Fmex: {
		OrderTypeLimit:  `limit`,
		OrderTypeMarket: `market`,
		OrderSideBuy:    `long`,
		OrderSideSell:   `short`,
	},
}

var orderStatusMap = map[string]map[string]string{ // market - market status - united status
	Binance: {
		"NEW":              CarryStatusWorking,
		"PARTIALLY_FILLED": CarryStatusWorking,
		"PENDING_CANCEL":   CarryStatusWorking,
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
		`pending_cancel`:   CarryStatusWorking, //撤销已提交
	},
	Fmex: {
		`PENDING`:           CarryStatusWorking, //	等待成交
		`PARTIAL_FILLED`:    CarryStatusWorking, //	部分成交
		`FULLY_FILLED`:      CarryStatusSuccess, //	完全成交
		`PARTIAL_CANCELLED`: CarryStatusSuccess, //	部分取消
		`FULLY_CANCELLED`:   CarryStatusFail,    //	全部取消
		`STOP_PENDING`:      CarryStatusWorking, //	stop订单正在等待触发
		`STOP_FAILED`:       CarryStatusFail,    //	stop订单被触发，但执行失败（例如：冻结失败）
		`STOP_CANCELLED`:    CarryStatusFail,    //	stop订单未被触发而取消
	},
	Bitmex: {
		"New":             CarryStatusWorking,
		"PartiallyFilled": CarryStatusWorking,
		"Filled":          CarryStatusSuccess,
		"DoneForDay":      CarryStatusWorking,
		"Canceled":        CarryStatusFail,
		"PendingCancel":   CarryStatusWorking,
		"Stopped":         CarryStatusFail,
		"Rejected":        CarryStatusFail,
		"PendingNew":      CarryStatusWorking,
		"Expired":         CarryStatusFail,
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
	Bybit: {
		`Created`:         CarryStatusWorking,
		`New`:             CarryStatusWorking,
		`PartiallyFilled`: CarryStatusWorking,
		`Filled`:          CarryStatusSuccess,
		`Cancelled`:       CarryStatusFail,
		`Rejected`:        CarryStatusFail,
		`PendingCancel`:   CarryStatusWorking,
		`Deactivated`:     CarryStatusFail,
	},
}

func SetCarryInfo(key, value string) {
	infoLock.Lock()
	defer infoLock.Unlock()
	if CarryInfo == nil {
		CarryInfo = make(map[string]string)
	}
	CarryInfo[key] = value
}

func GetBtcBalance(market string, timeBalance time.Time) (balance float64) {
	infoLock.Lock()
	defer infoLock.Unlock()
	if btcBalance == nil {
		btcBalance = make(map[string]float64)
	}
	return btcBalance[market+timeBalance.Format(time.RFC3339)[0:19]]
}

func SetBtcBalance(market string, timeBalance time.Time, balance float64) {
	infoLock.Lock()
	defer infoLock.Unlock()
	if btcBalance == nil {
		btcBalance = make(map[string]float64)
	}
	btcBalance[market+timeBalance.Format(time.RFC3339)[0:19]] = balance
}

func GetCandle(market, symbol, period, utcDate string) (candle *Candle) {
	infoLock.Lock()
	defer infoLock.Unlock()
	if candles == nil {
		candles = make(map[string]*Candle)
	}
	key := market + symbol + period + utcDate
	return candles[key]
}

func SetCandle(market, symbol, period, utcDate string, candle *Candle) {
	infoLock.Lock()
	defer infoLock.Unlock()
	if candles == nil {
		candles = make(map[string]*Candle)
	}
	key := market + symbol + period + utcDate
	candles[key] = candle
}

func GetFundingRate(market, symbol string) (rate float64, updateTime int64) {
	infoLock.Lock()
	defer infoLock.Unlock()
	if fundingRate == nil {
		fundingRate = make(map[string]map[string]float64)
	}
	if fundingRate[market] == nil {
		fundingRate[market] = make(map[string]float64)
	}
	if fundingRateUpdate == nil {
		fundingRateUpdate = make(map[string]map[string]int64)
	}
	if fundingRateUpdate[market] == nil {
		fundingRateUpdate[market] = make(map[string]int64)
	}
	return fundingRate[market][symbol], fundingRateUpdate[market][symbol]
}

func SetFundingRate(market, symbol string, rate float64, updateTime int64) {
	infoLock.Lock()
	defer infoLock.Unlock()
	if fundingRate == nil {
		fundingRate = make(map[string]map[string]float64)
	}
	if fundingRate[market] == nil {
		fundingRate[market] = make(map[string]float64)
	}
	if fundingRateUpdate == nil {
		fundingRateUpdate = make(map[string]map[string]int64)
	}
	if fundingRateUpdate[market] == nil {
		fundingRateUpdate[market] = make(map[string]int64)
	}
	fundingRate[market][symbol] = rate
	fundingRateUpdate[market][symbol] = updateTime
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

func GetWSSubscribe(market, symbol, subType string) (subscribe interface{}) {
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
		if len(symbol) > 4 && symbol[0:4] == `bch_` {
			symbol = `bchabc_` + symbol[4:]
		}
		return strings.ToLower(strings.Replace(symbol, "_", "", 1)) + `@depth5`
	case Fcoin:
		if subType == SubscribeDeal {
			// btc_usdt: trade.btcusdt
			return `trade.` + strings.ToLower(strings.Replace(symbol, "_", "", 1))
		} else if subType == SubscribeDepth {
			// btc_usdt: depth.L20.btcusdt
			return `depth.L20.` + strings.ToLower(strings.Replace(symbol, "_", "", 1))
		}
	case Fmex:
		if subType == SubscribeDepth {
			// btc_usdt: depth.L20.btcusdt
			return `depth.L20.` + symbol
		} else if subType == SubscribeDeal {
			return `trade.` + strings.ToUpper(symbol)
		}
	case Coinpark: //BTC_USDT bibox_sub_spot_BTC_USDT_ticker
		//return `bibox_sub_spot_` + strings.ToUpper(symbol) + `_ticker`
		return `bibox_sub_spot_` + strings.ToUpper(symbol) + `_depth`
	case Btcdo:
		return strings.ToUpper(symbol)
	case Bitmex:
		if subType == SubscribeDeal {
			return `trade:` + DialectSymbol[Bitmex][symbol]
		} else if subType == SubscribeDepth {
			//return `quote:` + DialectSymbol[Bitmex][symbol]
			//return `orderBookL2:` + DialectSymbol[Bitmex][symbol]
			//return `orderBookL2_25:` + DialectSymbol[Bitmex][symbol]
			return `orderBook10:` + DialectSymbol[Bitmex][symbol]
		}
		return ``
	case Bybit:
		subSymbol := strings.ToUpper(symbol[0:strings.Index(symbol, `_`)])
		if subType == SubscribeDeal {
			return `trade.` + subSymbol
		} else if subType == SubscribeDepth {
			//return `orderBook_200.100ms.` + subSymbol
			return `orderBookL2_25.` + subSymbol
		}
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
	for _, currency := range Currencies {
		if strings.Contains(original, currency) && strings.LastIndex(original, currency)+len(currency) == len(original) {
			return original[0:strings.LastIndex(original, currency)] + split + currency
		}
	}
	util.Notice(`can not parse symbol for currency absent ` + original)
	return ``
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
	case Fcoin: // btc_usdt: depth.L20.btcusdt  btc_usdt: trade.btcusdt
		if strings.Contains(subscribe, `depth`) {
			subscribe = strings.Replace(subscribe, "depth.L20.", "", 1)
			return getSymbolWithSplit(subscribe, "_")
		}
		if strings.Contains(subscribe, `trade`) {
			subscribe = strings.Replace(subscribe, `trade.`, ``, 1)
			return getSymbolWithSplit(subscribe, `_`)
		}
	case Fmex:
		subscribe = strings.Replace(subscribe, "depth.l20.", "", 1)
		return subscribe
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

func NewConfig() {
	AppConfig = &Config{}
	err := configor.Load(AppConfig, "./config.yml")
	if err != nil {
		util.Notice(err.Error())
		return
	}
	AppConfig.WSUrls = make(map[string]string)
	AppConfig.RestUrls = make(map[string]string)
	//AppConfig.WSUrls[Huobi] = "wss://api.huobi.pro/ws"
	AppConfig.WSUrls[Huobi] = `wss://api.huobi.br.com/ws`
	AppConfig.WSUrls[OKEX] = "wss://real.okex.com:10441/websocket?compress=true"
	AppConfig.WSUrls[OKFUTURE] = `wss://real.okex.com:10440/websocket?compress=true`
	AppConfig.WSUrls[Binance] = "wss://stream.binance.com:9443/stream?streams="
	AppConfig.WSUrls[Fcoin] = "wss://api.fcoin.com/v2/ws"
	if AppConfig.Env == `test` {
		AppConfig.WSUrls[Fmex] = `wss://api.testnet.fmex.com/v2/ws`
		AppConfig.RestUrls[Fmex] = `https://api.testnet.fmex.com/`
		//AppConfig.WSUrls[Bybit] = `wss://stream-testnet.bybit.com/realtime`
		//AppConfig.RestUrls[Bybit] = `https://api-testnet.bybit.com`
		AppConfig.WSUrls[Bybit] = `wss://stream.bybit.com/realtime`
		AppConfig.RestUrls[Bybit] = `https://api.bybit.com`
	} else {
		AppConfig.WSUrls[Fmex] = `wss://api.fmex.com/v2/ws`
		AppConfig.RestUrls[Fmex] = `https://api.fmex.com/`
		AppConfig.WSUrls[Bybit] = `wss://stream.bybit.com/realtime`
		AppConfig.RestUrls[Bybit] = `https://api.bybit.com`
	}
	AppConfig.WSUrls[Coinbig] = "wss://ws.coinbig.com/ws"
	AppConfig.WSUrls[Coinpark] = "wss://push.coinpark.cc/"
	AppConfig.WSUrls[Btcdo] = `wss://onli-quotation.btcdo.com/v1/market/?EIO=3&transport=websocket`
	AppConfig.WSUrls[Bitmex] = `wss://www.bitmex.com/realtime/`
	//AppConfig.WSUrls[Bitmex] = `wss://testnet.bitmex.com/realtime`
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
	//AppConfig.RestUrls[Bitmex] = `https://testnet.bitmex.com`
	AppConfig.RestUrls[Bitmex] = `https://www.bitmex.com/api/v1`
	AppConfig.MarketCost = make(map[string]float64)
	AppConfig.MarketCost[Fcoin] = 0
	AppConfig.MarketCost[Huobi] = 0.0005
	AppConfig.MarketCost[OKEX] = 0.0005
	AppConfig.MarketCost[OKFUTURE] = 0.0005
	AppConfig.MarketCost[Binance] = 0.0004
	AppConfig.MarketCost[Coinpark] = 0
	AppConfig.MarketCost[Coinbig] = 0
	AppConfig.MarketCost[Bitmex] = 0.0005
	AppConfig.SymbolPrice = make(map[string]float64)
	AppConfig.UpdatePriceTime = make(map[string]int64)
}

func GetAccountInfoSubscribe(marketName string) []interface{} {
	switch marketName {
	case OKFUTURE:
		//return []string{`ok_sub_futureusd_userinfo`}
		result := make([]interface{}, 1)
		result[0] = `login`
		return result
	}
	return nil
}

func (config *Config) SetSymbolPrice(symbol string, price float64) {
	config.lock.Lock()
	defer config.lock.Unlock()
	config.SymbolPrice[symbol] = price
}

func (config *Config) SetUpdatePriceTime(symbol string, updateTime int64) {
	config.lock.Lock()
	defer config.lock.Unlock()
	config.UpdatePriceTime[symbol] = updateTime
}

func (config *Config) ToString() string {
	str := "markets-carry cost:\n"
	for key, value := range config.MarketCost {
		str += fmt.Sprintf("-%s base carry cost: %f\n", key, value)
	}
	str += fmt.Sprintf("carry distance: %f\n", config.CarryDistance)
	str += fmt.Sprintf("delay: %f\n", config.Delay)
	str += fmt.Sprintf("channelslot: %f\n", config.ChannelSlot)
	str += fmt.Sprintf("minusdt: %f maxusdt: %f \n", config.MinUsdt, config.MaxUsdt)
	str += fmt.Sprintf("PreDealDis: %f Binance order dis: %f\n", config.PreDealDis, config.BinanceOrderDis)
	str += fmt.Sprintf("channels: %d \n", config.Channels)
	str += fmt.Sprintf("handle: %s handleMaker: %s handlerefresh: %s handlegrid: %s\n",
		config.Handle, config.HandleMaker, config.HandleRefresh, config.HandleGrid)
	str += fmt.Sprintf("orderwait: %d amountrate: %f\n", config.OrderWait, config.AmountRate)
	str += fmt.Sprintf("between: %d \n", config.Between)
	return str
}

func GetWSSubscribes(market, subType string) []interface{} {
	symbols := GetMarketSymbols(market)
	subscribes := make([]interface{}, 0)
	for symbol := range symbols {
		subTypes := strings.Split(subType, `,`)
		for _, value := range subTypes {
			subscribe := GetWSSubscribe(market, symbol, value)
			if subscribe != `` {
				subscribes = append(subscribes, subscribe)
			}
		}
		setting := GetSetting(FunctionMaker, market, symbol)
		if setting != nil {
			subscribes = append(subscribes, GetWSSubscribe(market, symbol, SubscribeDeal))
		}
	}
	if market == Bitmex || market == Bybit {
		//subscribes = append(subscribes, `order`)
		subscribes = append(subscribes, `position`)
	}
	return subscribes
}
