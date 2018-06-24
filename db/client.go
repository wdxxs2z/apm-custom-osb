package db

import (
	"fmt"
	"time"
	"golang.org/x/net/context"
	"code.cloudfoundry.org/lager"
	"github.com/wdxxs2z/apm-custom-osb/config"
	etcd "github.com/coreos/etcd/clientv3"
)

func open(config config.Config, logger lager.Logger) (*etcd.Client, error){

	dbConfig := etcd.Config{
		Endpoints:        	config.DatabaseConfig.Endpoints,
		DialTimeout:    	config.DatabaseConfig.DialTimeout * time.Second,
	}

	client, err := etcd.New(dbConfig)
	if err != nil {
	logger.Error("create-etcd-client-error", err, lager.Data{})
		return nil, err
	}

	return client, nil
}

func Exist(storeKey string, logger lager.Logger, config config.Config) (bool, error) {
	key := config.DatabaseConfig.DbName + "/" + storeKey
	logger.Debug("database-exist", lager.Data{
		"data-key": key,
	})
	client, err := open(config, logger)
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.DatabaseConfig.DialTimeout * time.Second)
	res, clientErr := client.Get(ctx, key)
	if clientErr != nil {
		return false, clientErr
	}
	cancel()
	defer client.Close()
	fmt.Println(res.Count)
	if res.Count < 1 {
		return false, nil
	}
	return true, nil
}

func CreateData(storeKey string, data string, logger lager.Logger, config config.Config) error {
	requestKey := config.DatabaseConfig.DbName + "/" + storeKey
	logger.Debug("database-create-data", lager.Data{
		"data-key": requestKey,
	})

	client, err := open(config, logger)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.DatabaseConfig.DialTimeout * time.Second)
	_, err = client.Put(ctx, requestKey, data)
	cancel()
	defer client.Close()
	if err != nil {
		return err
	}
	return nil
}

func GetData(storeKey string, logger lager.Logger, config config.Config) ([]byte, error) {
	requestKey := config.DatabaseConfig.DbName + "/" + storeKey
	logger.Debug("database-get-data", lager.Data{
		"data-key": requestKey,
	})

	client, err := open(config, logger)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.DatabaseConfig.DialTimeout * time.Second)
	resp, err := client.Get(ctx, requestKey)
	cancel()
	defer client.Close()
	if err != nil {
		return nil, err
	}
	var data []byte
	if resp.Count > 0 {
		for _, v := range resp.Kvs {
			data = v.Value
		}
		return data, nil
	} else {
		return nil, fmt.Errorf("db has no value with %s", requestKey)
	}
}

func UpdateData(storeKey string, data string, logger lager.Logger, config config.Config) error {
	requestKey := config.DatabaseConfig.DbName + "/" + storeKey
	logger.Debug("database-update-data", lager.Data{
		"data-key": requestKey,
	})

	client, err := open(config, logger)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.DatabaseConfig.DialTimeout * time.Second)
	_, err = client.Put(ctx, requestKey, data)
	cancel()
	defer client.Close()
	if err != nil {
		return err
	}
	return nil
}

func DeleteKey(storeKey string, logger lager.Logger, config config.Config) error {
	requestKey := config.DatabaseConfig.DbName + "/" + storeKey
	logger.Debug("database-delete-key", lager.Data{
		"data-key": requestKey,
	})

	client, err := open(config, logger)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.DatabaseConfig.DialTimeout * time.Second)
	_, err = client.Delete(ctx, requestKey, etcd.OpOption(etcd.WithPrefix()))
	cancel()
	defer client.Close()
	if err != nil {
		return err
	}
	return nil
}