package main

import (
	"fmt"
	"github.com/jinzhu/configor"
	"hello/api"
	"hello/model"
	"hello/util"
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
	//accountRights, keepDeposit := api.GetCurrencyOkfuture(`eos`)
	//fmt.Println(accountRights)
	//fmt.Println(keepDeposit)
	timeSlot := `5min`
	size := int64(1560)
	kpointsFuture := api.GetKLineOkexFuture(`btc_this_week`, timeSlot, size)
	kpoints := api.GetKLineOkex(`btc_usdt`, timeSlot, size)
	percentage := 0.0
	for key, value := range kpointsFuture {
		futureTime := time.Unix(value.TS/1000, 0).Format("Mon Jan 2 2006-01-02 15:04:05")
		currentTime := time.Unix(kpoints[key].TS/1000, 0).Format("Mon Jan 2 2006-01-02 15:04:05")
		percentage += (value.EndPrice - kpoints[key].EndPrice) / kpoints[key].EndPrice
		fmt.Println(fmt.Sprintf(`%d: %s - %s = %f `, key, futureTime, currentTime,
			100*(value.EndPrice-kpoints[key].EndPrice)/kpoints[key].EndPrice))
	}
	percentage = 100 *  percentage / float64(size)
	fmt.Println(percentage)
}
