package refresh

import (
	"github.com/jinzhu/configor"
	"github.com/jinzhu/gorm"
	"hello/api"
	"hello/carry"
	"hello/model"
	"hello/util"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

var lastOrderTime int64
var ordering = false

func checkTime(begin, end int64) bool {
	now := util.GetNowUnixMillion()
	if now-lastOrderTime < begin {
		return false
	}
	if end <= begin || now-lastOrderTime > end {
		return true
	}
	if rand.Int63n(end-begin+1) > now-lastOrderTime {
		return false
	}
	return true
}

// 只取第一位
func calcAmount(originalAmount float64) (num float64, err error) {
	str := strconv.FormatFloat(originalAmount, 'f', -1, 64)
	bytes := []byte(str)
	startReplace := false
	for i, v := range bytes {
		if startReplace && v != '.' {
			bytes[i] = '0'
		}
		if v != '0' && v != '.' {
			startReplace = true
		}
	}
	return strconv.ParseFloat(string(bytes), 64)
}

var ProcessRefresh = func(carry *model.Carry) {
	if ordering == true {
		return
	}
	ordering = true
	if !checkTime(model.ApplicationConfig.OrderWait-1000, model.ApplicationConfig.OrderWait+1000) {
		return
	}
	lastOrderTime = util.GetNowUnixMillion()
	currencies := strings.Split(carry.Symbol, "_")
	account := model.ApplicationAccounts.GetAccount(carry.AskWeb, currencies[0])
	if account == nil {
		util.Notice(`nil account ` + carry.AskWeb + currencies[0])
		return
	}
	leftBalance := account.Free
	account = model.ApplicationAccounts.GetAccount(carry.BidWeb, currencies[1])
	if account == nil {
		util.Notice(`nil account ` + carry.BidWeb + currencies[1])
		return
	}
	if leftBalance > carry.Amount {
		leftBalance = carry.Amount
	}
	rightBalance := account.Free
	if leftBalance*carry.BidPrice > rightBalance {
		leftBalance = rightBalance / carry.BidPrice
	}
	carry.Amount, _ = calcAmount(carry.Amount)
	leftBalance, _ = calcAmount(leftBalance)
	strLeftBalance := strconv.FormatFloat(leftBalance, 'f', -1, 64)
	carry.AskPrice = carry.BidPrice + (carry.AskPrice-carry.BidPrice)*2/3
	carry.BidPrice = carry.BidPrice + (carry.AskPrice-carry.BidPrice)*2/3
	strAskPrice := strconv.FormatFloat(carry.AskPrice, 'f', -1, 64)
	strBidPrice := strconv.FormatFloat(carry.BidPrice, 'f', -1, 64)
	timeOk, _ := carry.CheckWorthCarryTime(model.ApplicationMarkets, model.ApplicationConfig)
	if !timeOk {
		util.SocketInfo(`get carry not on time` + carry.ToString())
		return
	} else {
		model.ApplicationMarkets.BidAsks[carry.Symbol][carry.AskWeb] = nil
		model.ApplicationMarkets.BidAsks[carry.Symbol][carry.BidWeb] = nil
		util.Notice(`get carry worth` + carry.ToString())
		go api.DoAsk(carry, strAskPrice, strLeftBalance)
		go api.DoBid(carry, strBidPrice, strLeftBalance)
	}
	ordering = false
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
