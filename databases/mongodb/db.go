/**
 * Copyright 2020 gd Author. All rights reserved.
 * Author: Chuck1024
 */

package mongodb

import (
	"context"
	"errors"
	"fmt"
	"github.com/chuck1024/gd/dlog"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/ini.v1"
	"strings"
	"sync"
	"time"
)

const (
	DefaultMaxActive    = 500
	DefaultMaxIdle      = 8
	DefaultIdleTimeout  = 300
	DefaultRetryTimes   = 3
	DefaultConnTimeout  = 400
	DefaultReadTimeout  = 700
	DefaultWriteTimeout = 500

	MongoCommonCostMax   = 20
	MongoCmd             = "mongo_cmd_%v"
	MongoCmdSlowCount    = "mongo_%v_slow_count"
	MongoNormalSlowCount = "mongo_common_slow_count"

	glMongoCall     = "mongo_call"
	glMongoCost     = "mongo_cost"
	glMongoCallFail = "mongo_call_fail"

	defaultConf = "conf/conf.ini"
)

type MongoConfig struct {
	Hosts           []string
	User            string
	Password        string
	DataBase        string
	ReplicaSet      string
	ConnTimeoutMs   int64
	SocketTimeoutMs int64
	WTimeoutMs      int64
	MaxPoolSize     int
	MinPoolSize     int
	MaxIdleTimeMs   int64
	W               int
	Journal         string // true false
	Safe            string // true false
}

type MongoClient struct {
	DbConfig   *MongoConfig
	DbConf     *ini.File
	DbConfPath string
	DataBase   string // mongodb 实例名

	client *mongo.Client

	startOnce sync.Once
	closeOnce sync.Once
}

func (m *MongoClient) Start() error {
	var err error
	m.startOnce.Do(func() {
		if m.DbConfig != nil {
			err = m.initWithMongoConfig(m.DbConfig)
		} else if m.DbConf != nil {
			err = m.initDbs(m.DbConf, m.DataBase)
		} else {
			if m.DbConfPath == "" {
				m.DbConfPath = defaultConf
			}

			err = m.initObjForMongoDb(m.DbConfPath)
		}
	})
	return err
}

func (m *MongoClient) Close() {
	m.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := m.client.Disconnect(ctx); err != nil {
			dlog.Warn("mongoClient close err, %v", err)
		}
		dlog.Info("mongoClient close finish")
	})
}

func (m *MongoClient) initObjForMongoDb(filePath string) error {
	dbConfRealPath := filePath
	if dbConfRealPath == "" {
		return errors.New("dbConf not set in g_cfg")
	}

	if !strings.HasSuffix(dbConfRealPath, ".ini") {
		return errors.New("dbConf not an ini file")
	}

	dbConf, err := ini.Load(dbConfRealPath)
	if err != nil {
		return err
	}

	if err = m.initDbs(dbConf, m.DataBase); err != nil {
		return err
	}
	return nil
}

func (m *MongoClient) initDbs(f *ini.File, db string) error {
	c := f.Section(fmt.Sprintf("%s.%s", "Mongo", db))
	hosts := c.Key("hosts").Strings(",")
	userName := c.Key("user").String()
	password := c.Key("password").String()
	replicaSet := c.Key("replicaSet").String()
	journal := c.Key("journal").String()
	safe := c.Key("safe").String()
	connTimeoutMs, _ := c.Key("connTimeoutMs").Int64()
	socketTimeoutMs, _ := c.Key("socketTimeoutMs").Int64()
	wTimeoutMs, _ := c.Key("wTimeoutMs").Int64()
	maxPoolSize, _ := c.Key("maxPoolSize").Int()
	minPoolSize, _ := c.Key("minPoolSize").Int()
	w, _ := c.Key("w").Int()
	maxIdleTimeMs, _ := c.Key("maxIdleTimeMs").Int64()

	mc := &MongoConfig{
		Hosts:           hosts,
		User:            userName,
		Password:        password,
		DataBase:        db,
		ReplicaSet:      replicaSet,
		ConnTimeoutMs:   connTimeoutMs,
		SocketTimeoutMs: socketTimeoutMs,
		WTimeoutMs:      wTimeoutMs,
		MaxPoolSize:     maxPoolSize,
		MinPoolSize:     minPoolSize,
		MaxIdleTimeMs:   maxIdleTimeMs,
		W:               w,
		Journal:         journal,
		Safe:            safe,
	}

	err := m.initWithMongoConfig(mc)
	if err != nil {
		return err
	}

	return nil
}

func (m *MongoClient) initWithMongoConfig(c *MongoConfig) error {
	if len(c.Hosts) == 0 {
		return errors.New("mongo Config No Hosts")
	}

	hostStr := strings.Join(c.Hosts, ",")

	var optionStr, connStr string
	if len(c.DataBase) == 0 {
		c.DataBase = "admin"
	}

	if len(c.ReplicaSet) > 0 {
		optionStr += fmt.Sprintf("replicaSet=%s", c.ReplicaSet)
	}

	if c.ConnTimeoutMs > 0 {
		optionStr += fmt.Sprintf("connectTimeoutMs=%d", c.ConnTimeoutMs)
	}

	if c.SocketTimeoutMs > 0 {
		optionStr += fmt.Sprintf("socketTimeoutMs=%d", c.SocketTimeoutMs)
	}

	if c.WTimeoutMs > 0 {
		optionStr += fmt.Sprintf("wTimeoutMs=%d", c.WTimeoutMs)
	}

	if c.MaxPoolSize > 0 {
		optionStr += fmt.Sprintf("maxpoolSize=%d", c.MaxPoolSize)
	}

	if c.MinPoolSize > 0 {
		optionStr += fmt.Sprintf("minpoolSize=%d", c.MinPoolSize)
	}

	if c.MaxIdleTimeMs > 0 {
		optionStr += fmt.Sprintf("maxIdleTimeMs=%d", c.MaxIdleTimeMs)
	}

	if c.W > 0 {
		optionStr += fmt.Sprintf("w=%d", c.W)
	}

	if len(c.Journal) > 0 {
		optionStr += fmt.Sprintf("journal=%s", c.Journal)
	}

	if len(c.Safe) > 0 {
		optionStr += fmt.Sprintf("safe=%s", c.Safe)
	}

	if len(c.User) > 0 && len(c.Password) > 0 {
		connStr = fmt.Sprintf("mongodb://%s:%s@%s/%s?%s",
			c.User, c.Password, hostStr, c.DataBase, optionStr)
	} else {
		connStr = fmt.Sprintf("mongodb://%s/%s?%s",
			hostStr, c.DataBase, optionStr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(connStr))
	if err != nil {
		return err
	}

	m.client = client
	return nil
}

func (m *MongoClient) Insert(collection string, data []interface{}) ([]interface{}, error) {
	insertManyResult, err := m.client.Database(m.DataBase).Collection(collection).InsertMany(context.TODO(), data)
	if err != nil {
		dlog.Error("mongoClient Insert occur error:%v, collection:%s", err, collection)
		return nil, err
	}

	return insertManyResult.InsertedIDs, nil
}

func (m *MongoClient) UpdateOne(collection string, data interface{}, filter interface{}) (interface{}, error) {
	updateResult, err := m.client.Database(m.DataBase).Collection(collection).UpdateOne(context.TODO(), filter, data)
	if err != nil {
		dlog.Error("mongoClient UpdateOne occur error:%v, collection:%s", err, collection)
		return nil, err
	}

	return updateResult.UpsertedID, nil
}

func (m *MongoClient) UpdateMany(collection string, data interface{}, filter interface{}) (interface{}, error) {
	updateResult, err := m.client.Database(m.DataBase).Collection(collection).UpdateMany(context.TODO(), filter, data)
	if err != nil {
		dlog.Error("mongoClient UpdateMany occur error:%v, collection:%s", err, collection)
		return nil, err
	}

	return updateResult.UpsertedID, nil
}

func (m *MongoClient) DeleteOne(collection string, filter interface{}) (int64, error) {
	deleteResult, err := m.client.Database(m.DataBase).Collection(collection).DeleteOne(context.TODO(), filter)
	if err != nil {
		dlog.Error("mongoClient DeleteOne occur error:%v, collection:%s", err, collection)
		return 0, err
	}

	return deleteResult.DeletedCount, nil
}

func (m *MongoClient) DeleteMany(collection string, filter interface{}) (int64, error) {
	deleteResult, err := m.client.Database(m.DataBase).Collection(collection).DeleteMany(context.TODO(), filter)
	if err != nil {
		dlog.Error("mongoClient DeleteMany occur error:%v, collection:%s", err, collection)
		return 0, err
	}

	return deleteResult.DeletedCount, nil
}

func (m *MongoClient) FindOne(dataType interface{}, collection string, filter interface{}) error {
	result := m.client.Database(m.DataBase).Collection(collection).FindOne(context.TODO(), filter)

	if err := result.Decode(&dataType); err != nil {
		dlog.Error("mongoClient FindOne occur error:%v, collection:%s", err, collection)
		return err
	}
	return  nil
}

func (m *MongoClient) Find(dataType interface{}, collection string, filter interface{}, opts ...*options.FindOptions) (*mongo.Cursor, error) {
	cur, err := m.client.Database(m.DataBase).Collection(collection).Find(context.TODO(), filter, opts...)
	if err != nil {
		dlog.Error("mongoClient Find occur error:%v, collection:%s", err, collection)
		return nil, err
	}

	for cur.Next(context.TODO()) {

	}

	return cur, nil
}
