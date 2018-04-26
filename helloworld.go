package main

import (
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"hello/util"
	"hello/api"
	"hello/model"
	"github.com/jinzhu/configor"
	"fmt"
)

func main() {
	util.SocketInfo("start init")
	model.NewConfig()
	err := configor.Load(model.ApplicationConfig, "./config.yml")
	if err != nil {
		print(err)
		return
	}
	model.SetApiKeys()
	model.ApplicationDB, err = gorm.Open("postgres", model.ApplicationConfig.DBConnection)
	if err != nil {
		util.SocketInfo(fmt.Sprint(err))
		return
	}
	defer model.ApplicationDB.Close()
	model.ApplicationDB.AutoMigrate(&model.Carry{})
	model.ApplicationDB.AutoMigrate(&model.Account{})

	util.SocketInfo("start making money")
	model.HuobiAccountId, _ = api.GetSpotAccountId(model.ApplicationConfig)
	api.GetAccountHuobi(model.ApplicationAccounts)
	api.GetAccountOkex(model.ApplicationAccounts)
	api.GetAccountBinance(model.ApplicationAccounts)
	go api.CarryDBHandlerServe()
	go api.AskUpdate()
	go api.BidUpdate()
	go api.AccountDBHandlerServe()
	go api.CarryProcessor()

	markets := model.NewMarkets()
	api.Maintain(markets, model.ApplicationConfig)

}
