package refresh

import (
	"time"
	"github.com/jinzhu/configor"
	"github.com/jinzhu/gorm"
	"hello/util"
	"hello/model"
	"hello/api"
	"hello/carry"
)

var ProcessRefresh = func(carry *model.Carry) {
	//currencies := strings.Split(carry.Symbol, "_")
	//leftBalance := 0.0
	//rightBalance := 0.0
	//account := model.ApplicationAccounts.GetAccount(carry.AskWeb, currencies[0])
	//if account == nil {
	//	util.Notice(`nil account ` + carry.AskWeb + currencies[0])
	//	return
	//}
	//leftBalance = account.Free
	//account = model.ApplicationAccounts.GetAccount(carry.BidWeb, currencies[1])
	//if account == nil {
	//	util.Notice(`nil account ` + carry.BidWeb + currencies[1])
	//	return
	//}
	//rightBalance = account.Free
	//priceInUsdt, _ := model.GetBuyPriceOkex(currencies[0] + "_usdt")
	//minAmount := model.ApplicationConfig.MinUsdt / priceInUsdt
	//maxAmount := model.ApplicationConfig.MaxUsdt / priceInUsdt
	//if carry.Amount > maxAmount {
	//	carry.Amount = maxAmount
	//}
	//if leftBalance > carry.Amount {
	//	leftBalance = carry.Amount
	//}
	//if leftBalance*carry.BidPrice > rightBalance {
	//	leftBalance = rightBalance / carry.BidPrice
	//}
	//planAmount, _ := calcAmount(carry.Amount)
	//carry.Amount = planAmount
	//leftBalance, _ = calcAmount(leftBalance)
	//strLeftBalance := strconv.FormatFloat(leftBalance, 'f', -1, 64)
	//strAskPrice := strconv.FormatFloat(carry.AskPrice, 'f', -1, 64)
	//strBidPrice := strconv.FormatFloat(carry.BidPrice, 'f', -1, 64)
	//
	//timeOk, _ := carry.CheckWorthCarryTime(model.ApplicationMarkets, model.ApplicationConfig)
	//marginOk, _ := carry.CheckWorthCarryMargin(model.ApplicationMarkets, model.ApplicationConfig)
	//if !carry.CheckWorthSaveMargin() {
	//	// no need to save carry with margin < base cost
	//	return
	//}
	//doCarry := false
	//if !timeOk {
	//	carry.DealAskStatus = `NotOnTime`
	//	carry.DealBidStatus = `NotOnTime`
	//	util.SocketInfo(`get carry not on time` + carry.ToString())
	//} else {
	//	if !marginOk {
	//		carry.DealAskStatus = `NotWorth`
	//		carry.DealBidStatus = `NotWorth`
	//		util.SocketInfo(`get carry no worth` + carry.ToString())
	//	} else {
	//		model.ApplicationMarkets.BidAsks[carry.Symbol][carry.AskWeb] = nil
	//		model.ApplicationMarkets.BidAsks[carry.Symbol][carry.BidWeb] = nil
	//		if leftBalance < minAmount {
	//			carry.DealAskStatus = `NotEnough`
	//			carry.DealBidStatus = `NotEnough`
	//			util.Info(fmt.Sprintf(`leftB %f rightB/bidPrice %f/%f NotEnough %f - %f %s`, account.Free,
	//				rightBalance, carry.BidPrice, leftBalance, minAmount, carry.ToString()))
	//		} else {
	//			if model.ApplicationConfig.Env == `test` {
	//				carry.DealAskStatus = `NotDo`
	//				carry.DealBidStatus = `NotDo`
	//			} else {
	//				util.Notice(`get carry worth` + carry.ToString())
	//				doCarry = true
	//			}
	//		}
	//	}
	//}
	//if doCarry {
	//	go api.DoAsk(carry, strAskPrice, strLeftBalance)
	//	go api.DoBid(carry, strBidPrice, strLeftBalance)
	//} else {
	//	model.BidChannel <- *carry
	//}
}

func Maintain() {
	util.Notice("start fcoin refreshing")
	model.NewConfig()
	err := configor.Load(model.ApplicationConfig, "./config.yml")
	if err != nil {
		print(err)
		return
	}
	model.SetApiKeys()
	model.ApplicationDB, err = gorm.Open("postgres", model.ApplicationConfig.DBConnection)
	if err != nil {
		util.Notice(err.Error())
		return
	}
	defer model.ApplicationDB.Close()
	model.ApplicationDB.AutoMigrate(&model.Carry{})
	model.ApplicationDB.AutoMigrate(&model.Account{})
	go api.RefreshAccounts()
	go carry.DBHandlerServe()
	go carry.AskUpdate()
	go carry.BidUpdate()
	go carry.AccountDBHandlerServe()
	go carry.MaintainOrders()
	model.ApplicationMarkets = model.NewMarkets()
	for true {
		go carry.MaintainMarketChan(ProcessRefresh)
		time.Sleep(time.Duration(model.ApplicationConfig.ChannelSlot) * time.Millisecond)
	}
}
