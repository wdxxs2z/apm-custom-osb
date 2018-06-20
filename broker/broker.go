package broker

import (
	"os"
	"log"
	"fmt"
	"context"
	"strings"
	"net/http"
	"encoding/json"

	"github.com/gorilla/mux"
	"github.com/gorilla/handlers"
	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi"

	"github.com/wdxxs2z/skywalking-osb/config"
	"github.com/wdxxs2z/skywalking-osb/db"
)

type ProvisionParameters map[string]string

type BindParameters map[string]interface{}

type SkyWalkingBroker struct {
	allowUserProvisionParameters 	bool
	allowUserUpdateParameters    	bool
	allowUserBindParameters      	bool
	logger                  	lager.Logger
	brokerRouter			*mux.Router
	config                          config.Config
	skbrokerRouter			*mux.Router
}

func New(config config.Config, logger lager.Logger) *SkyWalkingBroker{
	brokerRouter := mux.NewRouter()
	broker := &SkyWalkingBroker{
		allowUserBindParameters:	config.AllowUserBindParameters,
		allowUserProvisionParameters:   config.AllowUserProvisionParameters,
		allowUserUpdateParameters:      config.AllowUserUpdateParameters,
		logger:				logger.Session("broker-api"),
		brokerRouter:                   brokerRouter,
		config:                         config,
	}
	brokerapi.AttachRoutes(broker.brokerRouter, broker, logger)
	liveness := broker.brokerRouter.HandleFunc("/liveness", livenessHandler).Methods(http.MethodGet)

	broker.brokerRouter.Use(authHandler(config, map[*mux.Route]bool{liveness: true}))
	broker.brokerRouter.Use(handlers.ProxyHeaders)
	broker.brokerRouter.Use(handlers.CompressHandler)
	broker.brokerRouter.Use(handlers.CORS(
		handlers.AllowedOrigins([]string{"*"}),
		handlers.AllowedMethods([]string{http.MethodHead, http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions}),
		handlers.AllowCredentials(),
	))

	return broker
}

func (sb *SkyWalkingBroker)Run(address string)  {
	log.Println("Skywalking APM Service Broker started on port " + strings.TrimPrefix(address, ":"))
	log.Fatal(http.ListenAndServe(address, sb.brokerRouter))
}

func (sb *SkyWalkingBroker)Services(context context.Context) ([]brokerapi.Service, error){
	sb.logger.Debug("fetch-service-catalog",lager.Data{})

	skywalkingService := sb.config.Services[0]
	return []brokerapi.Service{
		brokerapi.Service{
		ID:			skywalkingService.Id,
		Name:           	skywalkingService.Name,
		Description:    	skywalkingService.Description,
		Bindable:       	skywalkingService.Bindable,
		Tags:           	skywalkingService.Tags,
		PlanUpdatable:  	skywalkingService.PlanUpdateable,
		Plans:          	[]brokerapi.ServicePlan{
			brokerapi.ServicePlan{
				ID:		skywalkingService.Plans[0].Id,
				Name:           skywalkingService.Plans[0].Name,
				Description:    skywalkingService.Plans[0].Description,
				Free:           skywalkingService.Plans[0].Free,
				Bindable:       skywalkingService.Plans[0].Bindable,
				Metadata:       &brokerapi.ServicePlanMetadata{
					DisplayName:		skywalkingService.Plans[0].Description,
					Bullets: 		skywalkingService.Plans[0].Metadata.Bullets,
				},
			},
		},
		Metadata:       	&brokerapi.ServiceMetadata{
			DisplayName:		skywalkingService.Metadata.DisplayName,
			ImageUrl:               skywalkingService.Metadata.ImageUrl,
			LongDescription:	skywalkingService.Metadata.LongDescription,
			ProviderDisplayName:    skywalkingService.Metadata.ProviderDisplayName,
			DocumentationUrl:	skywalkingService.Metadata.DocumentationUrl,
			SupportUrl:		skywalkingService.Metadata.SupportUrl,
			},
		},
	}, nil
}

func (sb *SkyWalkingBroker)Provision(context context.Context, instanceID string, details brokerapi.ProvisionDetails, asyncAllowed bool) (brokerapi.ProvisionedServiceSpec, error) {
	sb.logger.Debug("provision", lager.Data{
		"instance_id":        	instanceID,
	})
	exist, err := db.Exist(instanceID + "/details", sb.logger, sb.config)
	if err != nil {
		return brokerapi.ProvisionedServiceSpec{}, err
	}
	if exist {
		return brokerapi.ProvisionedServiceSpec{}, brokerapi.ErrInstanceAlreadyExists
	}
	data, err := json.Marshal(details)
	if err != nil {
		return brokerapi.ProvisionedServiceSpec{}, err
	}
	dbErr := db.CreateData(instanceID + "/details", string(data[:]), sb.logger, sb.config)
	if dbErr != nil {
		return brokerapi.ProvisionedServiceSpec{}, dbErr
	}
	return brokerapi.ProvisionedServiceSpec{}, nil
}

func (sb *SkyWalkingBroker)Deprovision(context context.Context, instanceID string, details brokerapi.DeprovisionDetails, asyncAllowed bool) (brokerapi.DeprovisionServiceSpec, error){
	sb.logger.Debug("deprovision", lager.Data{
		"instance_id":        	instanceID,
	})
	exist, existErr := db.Exist(instanceID + "/details", sb.logger, sb.config)
	if existErr != nil {
		return brokerapi.DeprovisionServiceSpec{}, existErr
	}
	if !exist {
		return brokerapi.DeprovisionServiceSpec{}, brokerapi.ErrInstanceDoesNotExist
	}
	err := db.DeleteKey(instanceID + "/", sb.logger, sb.config)
	if err != nil {
		return brokerapi.DeprovisionServiceSpec{}, err
	}
	return brokerapi.DeprovisionServiceSpec{}, nil
}

func (sb *SkyWalkingBroker)Bind(context context.Context, instanceID, bindingID string, details brokerapi.BindDetails) (brokerapi.Binding, error){
	sb.logger.Debug("bind", lager.Data{
		"instance_id":        	instanceID,
	})

	bindParameters := BindParameters{}
	credentials := make(map[string]interface{})

	instanceExist, instanceExistErr := db.Exist(instanceID + "/details", sb.logger, sb.config)
	if instanceExistErr != nil {
		return brokerapi.Binding{}, instanceExistErr
	}
	if !instanceExist {
		return brokerapi.Binding{}, brokerapi.ErrInstanceDoesNotExist
	}

	exist, err := db.Exist(instanceID + "/bindings/" + bindingID + "/credentials", sb.logger, sb.config)
	if err != nil {
		return brokerapi.Binding{}, err
	}
	if exist {
		if sb.allowUserBindParameters && len(details.GetRawParameters()) >0 {
			if err := json.Unmarshal(details.GetRawParameters(), &bindParameters); err != nil {
				return brokerapi.Binding{}, err
			}
			parameters, err := validateParameter(bindParameters)
			if err != nil {
				return brokerapi.Binding{}, err
			}
			parameters["servers"] = sb.config.SkyWalkingConfig.Servers
			data, jsonErr := json.Marshal(parameters)
			if jsonErr != nil {
				return brokerapi.Binding{}, jsonErr
			}
			db.UpdateData(instanceID + "/bindings/" + bindingID + "/credentials", string(data[:]), sb.logger, sb.config)
			return brokerapi.Binding{
				Credentials:		parameters,
			}, nil
		}
		data, err := db.GetData(instanceID + "/bindings/" + bindingID + "/credentials", sb.logger, sb.config)
		if err != nil {
			return brokerapi.Binding{}, err
		}
		if err := json.Unmarshal(data, &bindParameters); err != nil {
			return brokerapi.Binding{}, err
		}
		return brokerapi.Binding{
			Credentials:		bindParameters,
		}, nil
	}

	servers := sb.config.SkyWalkingConfig.Servers
	if sb.allowUserBindParameters && len(details.GetRawParameters()) >0 {
		if err := json.Unmarshal(details.GetRawParameters(), &bindParameters); err != nil {
			return brokerapi.Binding{}, err
		}
		parameters, err := validateParameter(bindParameters)
		if err != nil {
			return brokerapi.Binding{}, err
		}
		credentials = parameters
	}
	credentials["servers"] = servers
	data, jsonErr := json.Marshal(credentials)
	if jsonErr != nil {
		return brokerapi.Binding{}, jsonErr
	}

	if err := db.CreateData(instanceID + "/bindings/" + bindingID + "/credentials", string(data[:]), sb.logger, sb.config); err != nil {
		return brokerapi.Binding{}, err
	}
	return brokerapi.Binding{
		Credentials:		credentials,
	}, nil
}

func (sb *SkyWalkingBroker)Unbind(context context.Context, instanceID, bindingID string, details brokerapi.UnbindDetails) error {
	sb.logger.Debug("unbind", lager.Data{
		"instance_id":        	instanceID,
	})
	exist, existErr := db.Exist(instanceID + "/bindings/" + bindingID + "/credentials", sb.logger, sb.config)
	if existErr != nil {
		return existErr
	}
	if !exist {
		return brokerapi.ErrBindingDoesNotExist
	}
	err := db.DeleteKey(instanceID + "/bindings/" + bindingID + "/credentials", sb.logger, sb.config)
	if err != nil {
		return err
	}
	return nil
}

func (sb *SkyWalkingBroker)LastOperation(context context.Context, instanceID, operationData string) (brokerapi.LastOperation, error) {
	sb.logger.Debug("last-operation", lager.Data{
		"instance_id":        	instanceID,
	})
	return brokerapi.LastOperation{}, nil
}

func (sb *SkyWalkingBroker)Update(context context.Context, instanceID string, details brokerapi.UpdateDetails, asyncAllowed bool) (brokerapi.UpdateServiceSpec, error) {
	sb.logger.Debug("update", lager.Data{
		"instance_id":        	instanceID,
	})
	return brokerapi.UpdateServiceSpec{}, nil
}

//private function
func livenessHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("{}"))
}

func authHandler(config config.Config, noAuthRequired map[*mux.Route]bool) mux.MiddlewareFunc{
	validCredentials := func(r *http.Request) bool {
		if noAuthRequired[mux.CurrentRoute(r)] {
			return true
		}
		user := os.Getenv("USERNAME")
		pass := os.Getenv("PASSWORD")
		username, password, ok := r.BasicAuth()
		if ok && username == user && password == pass {
			return true
		}
		return false
	}

	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !validCredentials(r) {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			handler.ServeHTTP(w, r)
		})
	}
}

func validateParameter(parameters map[string]interface{}) (map[string]interface{}, error) {
	credentials := make(map[string]interface{})
	for param, value := range parameters {
		if strings.EqualFold(param, "span-limit-per-segment"){
			if _,b := value.(float64); b {
				credentials[param] = value
			}else {
				return nil, fmt.Errorf("Error parameter %s set,error is: %s", param, "not int type")
			}
		}
		if strings.EqualFold(param, "ignore-suffix") {
			if _,b := value.(string); b {
				credentials[param] = value
			}else {
				return nil, fmt.Errorf("Error set %s parameter,must be string,such as:'.html'", param)
			}
		}
		if strings.EqualFold(param, "is-open-debugging-class") {
			if _, b:= value.(bool); b {
				credentials[param] = value
			}else {
				return nil, fmt.Errorf("Error set %s,must be bool", param)
			}
		}
		if strings.EqualFold(param, "logging-level") {
			if _, b := value.(string); b {
				if strings.Contains(value.(string), "DEBUG") || strings.Contains(value.(string), "INFO") || strings.Contains(value.(string), "ERROR") || strings.Contains(value.(string), "WARNING") {
					credentials[param] = value
				}else {
					return nil, fmt.Errorf("Error set %s, must set DEBUG,INFO,ERROR,WARNING", param)
				}
			}else {
				return nil, fmt.Errorf("Error set %s, must set DEBUG,INFO,ERROR,WARNING string", param)
			}
		}
	}
	return credentials, nil
}