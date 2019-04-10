package carry

import (
	"fmt"
	"github.com/pkg/errors"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
)

var marketMaking bool
var makers = make(map[string][]*model.Order) // market - order array
var makersLock sync.Mutex

func setMarketMaking(making bool) {
	marketMaking = making
}

func CancelOldMakers() {
	for true {
		markets := model.GetFunctionMarkets(model.FunctionMaker)
		d, _ := time.ParseDuration("-3s")
		timeLine := util.GetNow().Add(d)
		array := make([]*model.Order, 0)
		for _, market := range markets {
			for _, value := range makers[market] {
				if value.OrderTime.Before(timeLine) {
					api.MustCancel(market, value.Symbol, value.OrderId, true)
				} else {
					array = append(array, value)
				}
			}
			makers[market] = array
		}
		time.Sleep(time.Second)
	}
}

func addMaker(market string, order *model.Order) {
	makersLock.Lock()
	defer makersLock.Unlock()
	if makers[market] == nil {
		array := make([]*model.Order, 1)
		array[0] = order
		makers[market] = array
	} else {
		makers[market] = append(makers[market], order)
	}
}

func getBalance(market, symbol, accountType string) (left, right float64, err error) {
	leverMarket := market
	if accountType == model.AccountTypeLever {
		leverMarket = fmt.Sprintf(`%s_%s_%s`, market, model.AccountTypeLever,
			strings.Replace(symbol, `_`, ``, 1))
	}
	coins := strings.Split(symbol, `_`)
	leftAccount := model.AppAccounts.GetAccount(leverMarket, coins[0])
	if leftAccount == nil {
		util.Notice(`nil account ` + market + coins[0])
		time.Sleep(time.Second * 2)
		api.RefreshAccount(market)
		return 0, 0, errors.New(`no left balance`)
	}
	rightAccount := model.AppAccounts.GetAccount(leverMarket, coins[1])
	if rightAccount == nil {
		util.Notice(`nil account ` + market + coins[1])
		time.Sleep(time.Second * 2)
		api.RefreshAccount(market)
		return 0, 0, errors.New(`no right balance`)
	}
	return leftAccount.Free, rightAccount.Free, nil
}

var ProcessMake = func(market, symbol string) {
	if model.AppConfig.Handle != `1` || model.AppConfig.HandleMaker != `1` || marketMaking {
		return
	}
	setMarketMaking(true)
	defer setMarketMaking(false)
	setting := model.GetSetting(model.FunctionMaker, market, symbol)
	params := strings.Split(setting.FunctionParameter, `_`)
	if len(params) != 2 {
		util.Notice(`maker param error: require d_d format param while get ` + setting.FunctionParameter)
		return
	}
	amount, err := strconv.ParseFloat(params[1], 64)
	deal := model.AppMarkets.GetBigDeal(symbol, market)
	if model.AppMarkets.BidAsks[symbol] == nil || model.AppMarkets.BidAsks[symbol][market] == nil ||
		len(model.AppMarkets.BidAsks[symbol][market].Bids) == 0 ||
		len(model.AppMarkets.BidAsks[symbol][market].Asks) == 0 || deal == nil {
		util.Notice(`nil bid-ask price for ` + symbol)
		return
	}
	tick := model.AppMarkets.BidAsks[symbol][market]
	dealDelay := util.GetNowUnixMillion() - int64(deal.Ts)
	depthDelay := util.GetNowUnixMillion() - int64(tick.Ts)
	if dealDelay > 1000 || depthDelay > 2000 {
		util.Notice(fmt.Sprintf(`[delay too long] depth:%d deal:%d`, depthDelay, dealDelay))
		return
	}
	left, right, err := getBalance(market, symbol, setting.AccountType)
	if err != nil {
		return
	}
	orderSide := ``
	priceDistance := 0.9 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	rightAmount := right / deal.Price
	inAll := 0.0
	if left < rightAmount && amount < rightAmount {
		for i := 0; i < tick.Asks.Len(); i++ {
			if tick.Asks[i].Price <= deal.Price+priceDistance {
				inAll += tick.Asks[i].Amount
			} else {
				break
			}
		}
		orderSide = model.OrderSideBuy
	} else if left > rightAmount && left > amount {
		for i := 0; i < tick.Bids.Len(); i++ {
			if tick.Bids[i].Price >= deal.Price-priceDistance {
				inAll += tick.Bids[i].Amount
			} else {
				break
			}
		}
		orderSide = model.OrderSideSell
	}
	if inAll >= 0.5*amount {
		util.Notice(fmt.Sprintf(`[maker price full]deal price:%f, amount in all%f`, deal.Price, inAll))
		orderSide = ``
	}
	if orderSide != `` {
		order := api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``,
			setting.AccountType, deal.Price, amount)
		order.Function = model.FunctionMaker
		time.Sleep(time.Millisecond * 500)
		api.RefreshAccount(market)
		if order.Status == model.CarryStatusWorking {
			addMaker(market, order)
		}
		if order.OrderId != `` {
			model.AppDB.Save(order)
		}
	}
}
