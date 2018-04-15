package util

import (
	"fmt"
	"log"
	"os"
	"strconv"
)

var socket, notice *log.Logger
var socketInfoFile, noticeFile *os.File
var socketCount, noticeCount int

//var info *log.Logger
//var infoFile *os.File
//var infoCount int

func initLog(path string) (*log.Logger, *os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0660)
	if err != nil {
		return nil, nil, err
	}
	return log.New(file, "", log.Ldate|log.Ltime), file, nil
}

func getPath(name string) string {
	year, month, date := GetNow().Date()
	strDate := strconv.Itoa(year) + month.String() + strconv.Itoa(date)
	strTime := strconv.Itoa(GetNow().Hour()) + "_" + strconv.Itoa(GetNow().Minute())
	return "./log/" + name + strDate + "_" + strTime + ".log"
}

func SocketInfo(message string) {
	fmt.Println(message)
	if socketCount%10000 == 0 {
		if socketInfoFile != nil {
			socketInfoFile.Close()
		}
		socket, socketInfoFile, _ = initLog(getPath("socketInfo"))
	}
	socket.Println(message)
	socketCount++
}

func Info(message string) {
	message = ""
	//fmt.Println(message)
	//if infoCount%10000 == 0 {
	//	if infoFile != nil {
	//		infoFile.Close()
	//	}
	//	info, infoFile, _ = initLog(getPath("info"))
	//}
	//info.Println(message)
	//infoCount++
}

func Notice(message string) {
	if noticeCount%10000 == 0 {
		if noticeFile != nil {
			noticeFile.Close()
		}
		notice, noticeFile, _ = initLog(getPath("notice"))
	}
	notice.Println(message)
	noticeCount++
}
