package main

import (
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"hello/util"
	"hello/api"
	"hello/model"
	"github.com/jinzhu/configor"
)

func main() {
	util.Notice("start application")
	model.ApplicationConfig = model.NewConfig()
	err := configor.Load(model.ApplicationConfig, "./config.yml")
	if err != nil {
		print(err)
		return
	}
	model.ApplicationDB, err = gorm.Open("postgres", model.ApplicationConfig.DBConnection)
	if err != nil {
		panic(err)
	}
	defer model.ApplicationDB.Close()
	model.ApplicationDB.AutoMigrate(&model.Carry{})
	model.ApplicationDB.AutoMigrate(&model.Account{})

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
