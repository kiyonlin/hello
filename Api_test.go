package main

import (
	"fmt"
	"github.com/jinzhu/configor"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"hello/api"
	"hello/model"
	"hello/util"
	"testing"
)

func Test_chan(t *testing.T) {
	c := make(chan int, 3)
	go func() {
		for i := 0; i < 10; i = i + 1 {
			c <- i
		}
		//close(c)
	}()
	for true {
		//j:=<- c
		//fmt.Println(j)
		<-c
		fmt.Println(`get one`)
	}
	fmt.Println("Finished")

}

func Test_loadOrders(t *testing.T) {
	model.NewConfig()
	_ = configor.Load(model.AppConfig, "./config.yml")
	var err error
	model.AppDB, err = gorm.Open("postgres", model.AppConfig.DBConnection)
	if err != nil {
		util.Notice(err.Error())
		return
	}
	//go carry.CheckPastRefresh()
	//for true {
	//	time.Sleep(time.Minute)
	//}
	//d, _ := time.ParseDuration("-24h")
	//timeLine := util.GetNow().Add(d)
	//before := util.GetNow().Unix()
	//after := timeLine.Unix()
	//orders := api.QueryOrders(model.Fcoin, `eos_usdt`, model.CarryStatusWorking, before, after)
	//for _, order := range orders {
	//	if order != nil && order.OrderId != `` {
	//		//result, errCode, msg := api.CancelOrder(market, symbol, order.OrderId)
	//		util.Notice(fmt.Sprintf(`[cancel old]%v %s %f`, true, order.OrderId, order.Price))
	//		time.Sleep(time.Millisecond * 100)
	//	}
	//}
	//api.QueryOrderDealsFcoin(`3BgqYy6o70gMlDiCgH0JJEEynoJPqYnz5SZSq-No0EhA2-D4pKe6BB0RqdfJ0fXTDCfKUfhBVHyAFphKAWwylA==`)
	//orders := api.QueryOrders(model.Fcoin, `btc_usdt`, `success`,
	//	1557529200, 1557504000)
	//for _, value := range orders {
	//	util.Notice(fmt.Sprintf(`,symbol:%s,%s,%s,%s,%s,%f,%f,%f,%f`,
	//		value.Symbol, value.OrderTime.String(), value.Function, value.OrderSide, value.Status,
	//		value.DealAmount, value.DealPrice, value.Fee, value.FeeIncome))
	//}
}

func Test_RefreshAccount(t *testing.T) {
	test := make(map[string]string)
	test[`sd`] = `sdf`
	test[`sd23`] = `sdf`
	fmt.Println(len(test))
	model.NewConfig()
	_ = configor.Load(model.AppConfig, "./config.yml")
	db, err := gorm.Open("postgres", model.AppConfig.DBConnection)
	if err != nil {
		util.Notice(err.Error())
		return
	}
	order := api.PlaceOrder(``, ``, model.OrderSideBuy, model.OrderTypeLimit, model.Fmex, `btcusd_p`, ``,
		model.AccountTypeLever, 6666, 1)
	db.Save(&order)
	cancelOrder := api.MustCancel(``, ``, model.Fmex, order.Symbol, order.OrderId, true)
	db.Save(&cancelOrder)
	orders := api.QueryOrders(`7fc67592435b416db6863d22d7e01799`, `6311bc12ca4645718103b7d0760f16b3`,
		model.Fmex, `btcusd_p`, ``, ``, 0, 0)
	fmt.Println(len(orders))
	api.MustCancel(`7fc67592435b416db6863d22d7e01799`, `6311bc12ca4645718103b7d0760f16b3`,
		model.Fmex, `btcusd_p`, `473630701910`, true)
	api.RefreshAccount(``, ``, model.Fmex)
	//perUsdt, _ := api.GetPrice(``, ``, `usdc_usdt`)
	newOrders := api.QueryOrders(``, ``, model.Fcoin, `xrp_usdt`,
		model.CarryStatusWorking, model.AccountTypeLever, 0, 0)
	fmt.Println(len(newOrders))
	//order := api.QueryOrderById(model.Fcoin, `ltc_usdt`,
	//	`pfWOAwuurFQMpmUfWmWd3rRCckHf0uGK_b6xI5tYuYPJArdNgsTMekQw7ppjspj7`)
	//fmt.Println(fmt.Sprintf(`%f %f %s`, order.Amount, order.DealAmount, order.Status))
	//_ = api.GetAccountOkfuture(model.AppAccounts)
	//for i := 0; i < 50; i++ {
	//	api.GetKLineOkexFuture(`btc_this_week`, `1min`, 100)
	//}
	//timeSlot := `1min`
	//size := int64(1560)
	//currency := `eos`
	//kpointsFuture := api.GetKLineOkexFuture(currency+`_this_week`, timeSlot, size)
	//kpoints := api.GetKLineOkex(currency+`_usdt`, timeSlot, size)
	//percentage := 0.0
	//ticks := make([]model.Tick, len(kpointsFuture))
	//i := 0
	//for key, value := range kpointsFuture {
	//	futureTime := time.Unix(value.TS/1000, 0).Format("Mon Jan 2 2006-01-02 15:04:05")
	//	currentTime := time.Unix(kpoints[key].TS/1000, 0).Format("Mon Jan 2 2006-01-02 15:04:05")
	//	percentage += (value.EndPrice - kpoints[key].EndPrice) / kpoints[key].EndPrice
	//	tick := model.Tick{Id: futureTime, Price: (value.EndPrice - kpoints[key].EndPrice) / kpoints[key].EndPrice}
	//	ticks[i] = tick
	//	i++
	//	fmt.Println(fmt.Sprintf(`%d: %s - %s = %f `, key, futureTime, currentTime,
	//		(value.EndPrice-kpoints[key].EndPrice)/kpoints[key].EndPrice))
	//
	//}
	//var sortedKLine model.Ticks
	//sortedKLine = ticks
	//percentage = 100 * percentage / float64(size)
	//fmt.Println(percentage)
	//sort.Sort(sortedKLine)
	//for _, value := range sortedKLine {
	//	fmt.Println(fmt.Sprintf(`%s %f`, value.Id, value.Price))
	//}
}
