package main

import (
	"github.com/jinzhu/configor"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"hello/util"
	"hello/api"
	"hello/model"
)

func main() {
	util.Notice("start application")
	db, err := gorm.Open("postgres", "host=172.29.0.3 port=5432 user=winston dbname=coincarry password=User@123 sslmode=disable")
	if err != nil {
		panic(err)
	}
	model.ApplicationDB = db
	defer model.ApplicationDB.Close()
	model.ApplicationDB.AutoMigrate(&model.Carry{})
	model.ApplicationDB.AutoMigrate(&model.Account{})

	model.ApplicationConfig = model.NewConfig()
	err = configor.Load(model.ApplicationConfig, "./config.yml")
	if err != nil {
		print(err)
		return
	}
	model.HuobiAccountId, _ = api.GetSpotAccountId(model.ApplicationConfig)
	go api.CarryDBHandlerServe()
	go api.AccountDBHandlerServe()
	go api.CarryProcessor()
	api.GetAccountHuobi(model.ApplicationAccounts)
	api.GetAccountOkex(model.ApplicationAccounts)
	api.GetAccountBinance(model.ApplicationAccounts)

	markets := model.NewMarkets()
	api.Maintain(markets, model.ApplicationConfig)

}
