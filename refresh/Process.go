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
		go carry.MaintainMarketChan()
		time.Sleep(time.Duration(model.ApplicationConfig.ChannelSlot) * time.Millisecond)
	}
}
