/**
 * Copyright 2019 godog Author. All rights reserved.
 * Author: Chuck1024
 */

package httplib

import (
	"encoding/json"
	"fmt"
	"github.com/bitly/go-simplejson"
	"github.com/chuck1024/doglog"
	"github.com/chuck1024/godog/utils"
	"github.com/gin-gonic/gin"
	"strconv"
	"strings"
	"time"
)

func GroupFilter() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		ret, _ := ParseRet(c)
		httpStatusInterface, _ := c.Get(CODE)
		httpStatus := httpStatusInterface.(int)
		c.JSON(httpStatus, ret)
	}
}

// example: log middle handle
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		st := time.Now()
		logId := strconv.FormatInt(st.UnixNano(), 10)
		c.Set(LogId, logId)
		realIp, _ := utils.GetRealIP(c.Request)
		c.Set(REMOTE_IP, realIp)

		c.Next()
		uri := c.Request.RequestURI
		uriSplits := strings.Split(uri, "?")
		path := uri
		if len(uriSplits) > 0 {
			path = uriSplits[0]
		}

		costDu := time.Now().Sub(st)
		cost := costDu / time.Millisecond

		data, ok := c.Get(DATA)
		if !ok {
			dataRaw, ok := c.Get(DATA_RAW)
			if ok {
				paramsBts, ok := dataRaw.([]byte)
				if !ok {
					data = fmt.Sprintf("%v", dataRaw)
				} else {
					data = string(paramsBts)
				}
			}
		}

		ret, _ := c.Get(RET)
		httpStatusInterface, _ := c.Get(CODE)
		httpStatus := httpStatusInterface.(int)

		handleErr, _ := c.Get(ERR)
		errStr := ""
		handleErrErr, ok := handleErr.(error)
		if ok {
			if handleErrErr != nil {
				errStr = handleErrErr.Error()
			}
		} else {
			if handleErr != nil {
				errStr = fmt.Sprintf("%v", handleErr)
			}
		}

		message := map[string]interface{}{
			"httpStatus": httpStatus,
			"cost":       strconv.FormatInt(int64(cost), 10) + "ms",
			"err":        errStr,
		}

		logIdObj, ok := c.Get(LogId)
		if ok {
			logIdStr, _ := logIdObj.(string)
			message["logId"] = logIdStr
		}

		ip, ok := c.Get(REMOTE_IP)
		if ok {
			IP, _ := ip.(string)
			message["ip"] = IP
		}

		dataByte, err := json.Marshal(data)
		if err != nil {
			doglog.Error("[Logger] data cant transfer to json ?! data is %v", data)
			message["data"] = data
		} else {
			datas, _ := simplejson.NewJson(dataByte)
			message["data"] = datas
		}
		retByte, err := json.Marshal(ret)
		if err != nil {
			doglog.Error("[Logger] ret cant transfer to json ?! ret is %v", ret)
			message["ret"] = ret
		} else {
			retsj, _ := simplejson.NewJson(retByte)
			message["ret"] = retsj
		}

		mj, jsonErr := utils.Marshal(message)
		if jsonErr != nil {
			doglog.Error("[Logger] marshal occur error")
		}

		if cost > 500 {
			doglog.Warn(fmt.Sprintf("%s [SESSION] %s", path, string(mj)))
			return
		}
		doglog.Info(fmt.Sprintf("%s [SESSION] %s", path, string(mj)))
	}
}
