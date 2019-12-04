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
	"time"
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
	model.NewConfig()
	_ = configor.Load(model.AppConfig, "./config.yml")
	today := util.GetNow()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	rate := api.GetFundingRate(model.Fmex, `btcusd_p`)
	fmt.Println(rate)
	rate = api.GetFundingRate(model.Fmex, `btcusd_p`)
	api.GetCandle(`HHCJIVMpxYEahfxGZH9NoFzD`, `T9PD2va1ovmiiZroFIqJnKL_k6ZLGC3hkay-hKrPiOROe_MY`, model.Bitmex,
		`btcusd_p`, `1d`, util.GetNow())
	api.GetBtcBalance(`HHCJIVMpxYEahfxGZH9NoFzD`, `T9PD2va1ovmiiZroFIqJnKL_k6ZLGC3hkay-hKrPiOROe_MY`, model.Bitmex)

	fmt.Println(rate)
	//order := api.QueryOrderById(`HHCJIVMpxYEahfxGZH9NoFzD`, `T9PD2va1ovmiiZroFIqJnKL_k6ZLGC3hkay-hKrPiOROe_MY`,
	//	model.Fmex, `btcusd_p`, `8330017131911`)
	//api.RefreshAccount(`HHCJIVMpxYEahfxGZH9NoFzD`, `T9PD2va1ovmiiZroFIqJnKL_k6ZLGC3hkay-hKrPiOROe_MY`, model.Bitmex)
	//order := api.QueryOrderById(`HHCJIVMpxYEahfxGZH9NoFzD`,
	//	`T9PD2va1ovmiiZroFIqJnKL_k6ZLGC3hkay-hKrPiOROe_MY`,
	//	model.Bitmex, `btcusd_p`, `c05bea31-932c-7d9b-2339-6d322e796b56`)
	//order := api.PlaceOrder(`82330bc7090f43a287994f0781f7ff58`, `6d5e010418d247b3a21dcc766566036d`,
	//	model.OrderSideBuy, model.OrderTypeLimit, model.Fmex, `btcusd_p`, ``, ``,
	//	7777, 1, true)

}
