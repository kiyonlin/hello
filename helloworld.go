package main

import (
	"github.com/jinzhu/configor"
	"hello/carry"
	"hello/controller"
	"hello/model"
	"hello/util"
)

func main() {
	model.NewConfig()
	err := configor.Load(model.AppConfig, "./config.yml")
	if err != nil {
		util.Notice(err.Error())
		return
	}
	go controller.ParameterServe()
	carry.Maintain()
	select {}
}
