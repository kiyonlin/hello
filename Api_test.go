package main

import (
	"fmt"
	"github.com/jinzhu/configor"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"hello/api"
	"hello/carry"
	"hello/model"
	"hello/util"
	"sort"
	"testing"
	"time"
)

func Test_RefreshAccount(t *testing.T) {
	model.NewConfig()
	err := configor.Load(model.AppConfig, "./config.yml")
	if err != nil {
		util.Notice(err.Error())
		return
	}
	_ = api.GetAccountOkfuture(model.AppAccounts)
	for i := 0; i < 50; i++ {
		api.GetKLineOkexFuture(`btc_this_week`, `1min`, 100)
	}
	timeSlot := `1min`
	size := int64(1560)
	currency := `eos`
	kpointsFuture := api.GetKLineOkexFuture(currency+`_this_week`, timeSlot, size)
	kpoints := api.GetKLineOkex(currency+`_usdt`, timeSlot, size)
	percentage := 0.0
	ticks := make([]model.Tick, len(kpointsFuture))
	i := 0
	for key, value := range kpointsFuture {
		futureTime := time.Unix(value.TS/1000, 0).Format("Mon Jan 2 2006-01-02 15:04:05")
		currentTime := time.Unix(kpoints[key].TS/1000, 0).Format("Mon Jan 2 2006-01-02 15:04:05")
		percentage += (value.EndPrice - kpoints[key].EndPrice) / kpoints[key].EndPrice
		tick := model.Tick{Id: futureTime, Price: (value.EndPrice - kpoints[key].EndPrice) / kpoints[key].EndPrice}
		ticks[i] = tick
		i++
		fmt.Println(fmt.Sprintf(`%d: %s - %s = %f `, key, futureTime, currentTime,
			(value.EndPrice-kpoints[key].EndPrice)/kpoints[key].EndPrice))

	}
	var sortedKLine model.Ticks
	sortedKLine = ticks
	percentage = 100 * percentage / float64(size)
	fmt.Println(percentage)
	sort.Sort(sortedKLine)
	for _, value := range sortedKLine {
		fmt.Println(fmt.Sprintf(`%s %f`, value.Id, value.Price))
	}
}

func Test_Api(t *testing.T) {
	model.NewConfig()
	err := configor.Load(model.AppConfig, "./config.yml")
	if err != nil {
		util.Notice(err.Error())
		return
	}
	model.AppDB, err = gorm.Open("postgres", model.AppConfig.DBConnection)
	if err != nil {
		util.Notice(err.Error())
		return
	}
	api.RefreshAccount(model.Fcoin)
	carry.MaintainTransFee()
	//order := api.PlaceOrder(model.OrderSideBuy, model.OrderTypeLimit, model.Fcoin, `btc_usdt`, ``,
	//	model.AccountTypeLever, 4999, 0.001)
	//fmt.Println(order.OrderId)
	//order := api.PlaceOrder(model.OrderSideSell, model.OrderTypeLimit, model.Fcoin, `bsv_usdt`, ``, 64.94, 30)
	//api.QueryOrder(order)
	//order := api.QueryOrderById(model.Fcoin, `xrp_btc`, `I_u7N8mADEnBchAtpdaTxrH-Tr8mJMDMA-MDOmVVr7oM2dOU-AOgzHjI0OG0Qhxv`)
	//fmt.Println(fmt.Sprintf(`status %s errcode %s`, order.Status, order.ErrCode))
	//testOrder := api.QueryOrderById(model.Fcoin, `eos_btc`, `X0-GKSE7iZkHEYoBfo7UmFEjhP8CfJsP8TiPPFymtWg9IKL4rIyhnz5KVvxWpNqQ`)
	//fmt.Println(testOrder.Status)
	////api.RefreshAccount(model.Fcoin)
	////orderId, errMsg, status, amount, price := api.PlaceOrder(model.OrderSideBuy, model.OrderTypeLimit, model.Fcoin,
	////	`eos_usdt`, model.AmountTypeCoinNumber, 2.263, 1)
	////fmt.Sprintf(`%s %s %s %f %f`, orderId, errMsg, status, amount, price)
	//orders := api.QueryOrders(model.Fcoin, `eos_usdt`, model.CarryStatusSuccess)
	//for key, value := range orders {
	//	order := api.QueryOrderById(model.Fcoin, value.Symbol, key)
	//	fmt.Println(order.OrderTime.String())
	//}
}

func Test_array(t *testing.T) {
	now := time.Now()
	nowsec := (now.Hour()*3600 + now.Minute()*60 + now.Second())
	slotNum := int(nowsec / model.RefreshTimeSlot)
	fmt.Println(slotNum)
	a, b := util.FormatNum(0.335, 2)
	fmt.Println(fmt.Sprintf(`%f %s`, a, b))
}
