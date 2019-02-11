package main

import (
	"hello/carry"
	"hello/controller"
)

func main() {
	go controller.ParameterServe()
	carry.Maintain()
	select {}
}
