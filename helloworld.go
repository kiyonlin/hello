package main

import (
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"hello/util"
	"hello/api"
	"hello/model"
	"github.com/jinzhu/configor"
	"time"
)

func refreshAccounts() {
	for true {
		api.GetAccountHuobi(model.ApplicationAccounts)
		api.GetAccountOkex(model.ApplicationAccounts)
		api.GetAccountBinance(model.ApplicationAccounts)
		api.GetAccountFcoin(model.ApplicationAccounts)
		time.Sleep(time.Minute * 1)
	}
}

func main() {
	util.Notice("start init")
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

	util.Notice("start making money")
	model.HuobiAccountId, _ = api.GetSpotAccountId(model.ApplicationConfig)
	go refreshAccounts()
	go api.CarryDBHandlerServe()
	go api.AskUpdate()
	go api.BidUpdate()
	go api.AccountDBHandlerServe()
	go api.CarryProcessor()
	model.ApplicationMarkets = model.NewMarkets()
	api.Maintain()
}
