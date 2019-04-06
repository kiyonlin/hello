package carry

import (
	"fmt"
	"github.com/pkg/errors"
	"hello/api"
	"hello/model"
	"hello/util"
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

func cancelOldMakers(market string) {
	makersLock.Lock()
	defer makersLock.Unlock()
	d, _ := time.ParseDuration("-5s")
	timeLine := util.GetNow().Add(d)
	array := make([]*model.Order, 0)
	for _, value := range makers[market] {
		if value.OrderTime.Before(timeLine) {
			api.MustCancel(market, value.Symbol, value.OrderId, true)
		} else {
			array = append(array, value)
		}
	}
	makers[market] = array
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
	if util.GetNowUnixMillion()-api.LastRefreshTime[market] > 15000 {
		util.Notice(`15 seconds past, refresh and return ` + market + symbol)
		api.RefreshAccount(market)
		return 0, 0, errors.New(`data older than 15 seconds`)
	}
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
	go cancelOldMakers(market)
	bidAsk := model.AppMarkets.BidAsks[symbol][market]
	deal := model.AppMarkets.GetDeal(symbol, market)
	if len(bidAsk.Asks) == 0 || bidAsk.Bids.Len() == 0 || deal == nil {
		return
	}
	delay := util.GetNowUnixMillion() - int64(model.AppMarkets.BidAsks[symbol][market].Ts)
	dealDelay := util.GetNowUnixMillion() - int64(deal.Ts)
	if delay > 200 || dealDelay > 1000 {
		util.Notice(fmt.Sprintf(`[delay too long] %d %d`, delay, dealDelay))
		return
	}
	setting := model.GetSetting(model.FunctionMaker, market, symbol)
	params := strings.Split(setting.FunctionParameter, `_`)
	if len(params) != 2 {
		util.Notice(`maker param error: require d_d format param while get ` + setting.FunctionParameter)
		return
	}
	bigOrderLine, errParam1 := strconv.ParseFloat(params[0], 64)
	amount, errParam2 := strconv.ParseFloat(params[1], 64)
	left, right, err := getBalance(market, symbol, setting.AccountType)
	if err != nil || errParam1 != nil || errParam2 != nil || bigOrderLine > deal.Amount {
		return
	}
	util.Notice(fmt.Sprintf(`[get big]%f:%f-%f %f_%f`, deal.Amount, amount, bigOrderLine, left, right/deal.Price))
	orderSide := ``
	if deal.Side == model.OrderSideBuy {
		if amount < right/deal.Price {
			orderSide = model.OrderSideSell
		} else if amount < left {
			orderSide = model.OrderSideBuy
		}
	} else if deal.Side == model.OrderSideSell {
		if amount < left {
			orderSide = model.OrderSideBuy
		} else if amount < right/deal.Price {
			orderSide = model.OrderSideSell
		}
	}
	if orderSide != `` {
		order := api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``,
			setting.AccountType, deal.Price, amount)
		if order.ErrCode == `1016` {
			api.RefreshAccount(market)
		}
		if order.Status == model.CarryStatusWorking {
			addMaker(market, order)
		}
	}
}
