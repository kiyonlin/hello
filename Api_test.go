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

func Test_initTurtleN(t *testing.T) {
	model.NewConfig()
	_ = configor.Load(model.AppConfig, "./config.yml")
	var err error
	model.AppDB, err = gorm.Open("postgres", model.AppConfig.DBConnection)
	if err != nil {
		util.Notice(err.Error())
		return
	}
	model.AppDB.AutoMigrate(&model.Candle{})
	today := time.Now().In(time.UTC)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	//api.GetDayCandle(`HHCJIVMpxYEahfxGZH9NoFzD`, `T9PD2va1ovmiiZroFIqJnKL_k6ZLGC3hkay-hKrPiOROe_MY`,
	//	model.Bitmex, `btcusd_p`, yesterday)
	fmt.Println(today.String())
	for i := 100; i > 0; i-- {
		d, _ := time.ParseDuration(fmt.Sprintf(`%dh`, -24*i))
		index := today.Add(d)
		fmt.Println(index.String())
		api.GetDayCandle(`HHCJIVMpxYEahfxGZH9NoFzD`, `T9PD2va1ovmiiZroFIqJnKL_k6ZLGC3hkay-hKrPiOROe_MY`,
			model.Bitmex, `btcusd_p`, index)
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
	var err error
	model.AppDB, err = gorm.Open("postgres", model.AppConfig.DBConnection)
	if err != nil {
		util.Notice(err.Error())
		return
	}
	carry.GetTurtleData(model.Bitmex, `btcusd_p`)
	//bitmexKey := `HHCJIVMpxYEahfxGZH9NoFzD`
	//bitmexSecret := `T9PD2va1ovmiiZroFIqJnKL_k6ZLGC3hkay-hKrPiOROe_MY`
	//api.PlaceOrder(bitmexKey, bitmexSecret,
	//	model.OrderSideSell, model.OrderTypeMarket, model.Bitmex, `btcusd_p`,
	//	``, ``, ``, 11111, 1, false)
}
