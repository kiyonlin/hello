package model

import "hello/util"

type TurtleStatus struct {
	LastDealPrice float64
	CarryTime     int64
	ExtraBid      float64
	ExtraAsk      float64
}

func GetTurtleStatus(market, symbol string) (status *TurtleStatus) {
	if ApplicationTurtleStatus == nil {
		ApplicationTurtleStatus = make(map[string]map[string]*TurtleStatus)
	}
	if ApplicationTurtleStatus[market] == nil {
		return nil
	}
	return ApplicationTurtleStatus[market][symbol]
}

func SetTurtleStatus(market, symbol string, turtleStatus *TurtleStatus) {
	if ApplicationTurtleStatus == nil {
		ApplicationTurtleStatus = make(map[string]map[string]*TurtleStatus)
	}
	if ApplicationTurtleStatus[market] == nil {
		ApplicationTurtleStatus[market] = make(map[string]*TurtleStatus)
	}
	turtleStatus.CarryTime = util.GetNowUnixMillion()
	ApplicationTurtleStatus[market][symbol] = turtleStatus
}
