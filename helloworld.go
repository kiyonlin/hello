package main

import (
	"hello/carry"
	"hello/controller"
)

func main() {
	carry.Maintain()
	go controller.ParameterServe()
	select {}
}
