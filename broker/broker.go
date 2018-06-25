package broker

import (
	"os"
	"log"
	"fmt"
	"context"
	"strings"
	"reflect"
	"net/http"
	"encoding/json"

	"github.com/gorilla/mux"
	"github.com/gorilla/handlers"
	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi"

	"github.com/wdxxs2z/apm-custom-osb/config"
	"github.com/wdxxs2z/apm-custom-osb/db"
	//"strconv"
)

type ProvisionParameters map[string]string

type BindParameters map[string]interface{}

type APMSserviceBroker struct {
	allowUserProvisionParameters 	bool
	allowUserUpdateParameters    	bool
	allowUserBindParameters      	bool
	logger                  	lager.Logger
	brokerRouter			*mux.Router
	config                          config.Config
}

func New(config config.Config, logger lager.Logger) *APMSserviceBroker{
	brokerRouter := mux.NewRouter()
	broker := &APMSserviceBroker{
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

func (sb *APMSserviceBroker)Run(address string)  {
	log.Println("APM Service Broker started on port " + strings.TrimPrefix(address, ":"))
	log.Fatal(http.ListenAndServe(address, sb.brokerRouter))
}

func (sb *APMSserviceBroker)Services(context context.Context) ([]brokerapi.Service, error){
	sb.logger.Debug("fetch-service-catalog",lager.Data{})

	apmServices := sb.config.Services

	var services []brokerapi.Service

	for _,apmService := range apmServices {

		services = append(services, brokerapi.Service{
			ID:			apmService.Id,
			Name:           	apmService.Name,
			Description:    	apmService.Description,
			Bindable:       	apmService.Bindable,
			Tags:           	apmService.Tags,
			PlanUpdatable:  	apmService.PlanUpdateable,
			Metadata:       	&brokerapi.ServiceMetadata{
				DisplayName:		apmService.Metadata.DisplayName,
				ImageUrl:               apmService.Metadata.ImageUrl,
				LongDescription:	apmService.Metadata.LongDescription,
				ProviderDisplayName:    apmService.Metadata.ProviderDisplayName,
				DocumentationUrl:	apmService.Metadata.DocumentationUrl,
				SupportUrl:		apmService.Metadata.SupportUrl,
			},
			Plans:          	servicePlans(apmService.Plans),

		})
	}
	return services,nil
}

func (sb *APMSserviceBroker)Provision(context context.Context, instanceID string, details brokerapi.ProvisionDetails, asyncAllowed bool) (brokerapi.ProvisionedServiceSpec, error) {
	sb.logger.Debug("provision", lager.Data{
		"instance_id":        	instanceID,
	})
	service,_ := sb.GetService(details.ServiceID)
	if service.Name == "" {
		return brokerapi.ProvisionedServiceSpec{}, fmt.Errorf("service (%s) not found in catalog", details.ServiceID)
	}
	exist, err := db.Exist(service.Name + "/" +instanceID + "/details", sb.logger, sb.config)
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
	dbErr := db.CreateData(service.Name + "/" +instanceID + "/details", string(data[:]), sb.logger, sb.config)
	if dbErr != nil {
		return brokerapi.ProvisionedServiceSpec{}, dbErr
	}
	return brokerapi.ProvisionedServiceSpec{}, nil
}

func (sb *APMSserviceBroker)Deprovision(context context.Context, instanceID string, details brokerapi.DeprovisionDetails, asyncAllowed bool) (brokerapi.DeprovisionServiceSpec, error){
	sb.logger.Debug("deprovision", lager.Data{
		"instance_id":        	instanceID,
	})
	service,_ := sb.GetService(details.ServiceID)
	if service.Name == "" {
		return brokerapi.DeprovisionServiceSpec{}, fmt.Errorf("service (%s) not found in catalog", details.ServiceID)
	}
	exist, existErr := db.Exist(service.Name + "/" + instanceID + "/details", sb.logger, sb.config)
	if existErr != nil {
		return brokerapi.DeprovisionServiceSpec{}, existErr
	}
	if !exist {
		return brokerapi.DeprovisionServiceSpec{}, brokerapi.ErrInstanceDoesNotExist
	}
	err := db.DeleteKey(service.Name + "/" + instanceID + "/", sb.logger, sb.config)
	if err != nil {
		return brokerapi.DeprovisionServiceSpec{}, err
	}
	return brokerapi.DeprovisionServiceSpec{}, nil
}

func (sb *APMSserviceBroker)Bind(context context.Context, instanceID, bindingID string, details brokerapi.BindDetails) (brokerapi.Binding, error){
	sb.logger.Debug("bind", lager.Data{
		"instance_id":        	instanceID,
	})

	bindParameters := BindParameters{}
	credentials := make(map[string]interface{})

	service,_ := sb.GetService(details.ServiceID)
	if service.Name == "" {
		return brokerapi.Binding{}, fmt.Errorf("service (%s) not found in catalog", details.ServiceID)
	}
	instanceExist, instanceExistErr := db.Exist(service.Name + "/" + instanceID + "/details", sb.logger, sb.config)
	if instanceExistErr != nil {
		return brokerapi.Binding{}, instanceExistErr
	}
	if !instanceExist {
		return brokerapi.Binding{}, brokerapi.ErrInstanceDoesNotExist
	}

	exist, err := db.Exist(service.Name + "/" + instanceID + "/bindings/" + bindingID + "/credentials", sb.logger, sb.config)
	if err != nil {
		return brokerapi.Binding{}, err
	}
	if exist {
		if sb.allowUserBindParameters && len(details.GetRawParameters()) >0 {
			if err := json.Unmarshal(details.GetRawParameters(), &bindParameters); err != nil {
				return brokerapi.Binding{}, err
			}
			parameters, err := sb.ValidateParameter(details.ServiceID, details.PlanID, bindParameters)
			if err != nil {
				return brokerapi.Binding{}, err
			}
			data, jsonErr := json.Marshal(parameters)
			if jsonErr != nil {
				return brokerapi.Binding{}, jsonErr
			}
			db.UpdateData(service.Name + "/" + instanceID + "/bindings/" + bindingID + "/credentials", string(data[:]), sb.logger, sb.config)
			return brokerapi.Binding{
				Credentials:		parameters,
			}, nil
		}
		data, err := db.GetData(service.Name + "/" + instanceID + "/bindings/" + bindingID + "/credentials", sb.logger, sb.config)
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

	if sb.allowUserBindParameters && len(details.GetRawParameters()) >0 {
		if err := json.Unmarshal(details.GetRawParameters(), &bindParameters); err != nil {
			return brokerapi.Binding{}, err
		}
		parameters, err := sb.ValidateParameter(details.ServiceID, details.PlanID, bindParameters)
		if err != nil {
			return brokerapi.Binding{}, err
		}
		credentials = parameters
	}
	data, jsonErr := json.Marshal(credentials)
	if jsonErr != nil {
		return brokerapi.Binding{}, jsonErr
	}

	if err := db.CreateData(service.Name + "/" + instanceID + "/bindings/" + bindingID + "/credentials", string(data[:]), sb.logger, sb.config); err != nil {
		return brokerapi.Binding{}, err
	}
	return brokerapi.Binding{
		Credentials:		credentials,
	}, nil
}

func (sb *APMSserviceBroker)Unbind(context context.Context, instanceID, bindingID string, details brokerapi.UnbindDetails) error {
	sb.logger.Debug("unbind", lager.Data{
		"instance_id":        	instanceID,
	})
	service,_ := sb.GetService(details.ServiceID)
	if service.Name == "" {
		return fmt.Errorf("service (%s) not found in catalog", details.ServiceID)
	}
	exist, existErr := db.Exist(service.Name + "/" + instanceID + "/bindings/" + bindingID + "/credentials", sb.logger, sb.config)
	if existErr != nil {
		return existErr
	}
	if !exist {
		return brokerapi.ErrBindingDoesNotExist
	}
	err := db.DeleteKey(service.Name + "/" + instanceID + "/bindings/" + bindingID + "/credentials", sb.logger, sb.config)
	if err != nil {
		return err
	}
	return nil
}

func (sb *APMSserviceBroker)LastOperation(context context.Context, instanceID, operationData string) (brokerapi.LastOperation, error) {
	sb.logger.Debug("last-operation", lager.Data{
		"instance_id":        	instanceID,
	})
	return brokerapi.LastOperation{}, nil
}

func (sb *APMSserviceBroker)Update(context context.Context, instanceID string, details brokerapi.UpdateDetails, asyncAllowed bool) (brokerapi.UpdateServiceSpec, error) {
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

func (sb *APMSserviceBroker)ValidateParameter(serviceId string, planId string, parameters map[string]interface{}) (map[string]interface{}, error) {
	credentials := make(map[string]interface{})
	plan,_:= sb.GetPlan(serviceId, planId)
	for pk,pv := range plan.Parameters {
		for param, value := range parameters {
			if strings.EqualFold(pk, param) {
				if reflect.TypeOf(value).Kind().String() == "float64" {
					credentials[pk] = value
				} else if (reflect.TypeOf(pv) == reflect.TypeOf(value)) {
					credentials[pk] = value
				} else {
					return nil, fmt.Errorf("Error parameter %s set,correct type is %s", pk, reflect.TypeOf(pv))
				}
			}
		}
		credentials[pk] = pv
	}
	return credentials, nil
}

func (sb *APMSserviceBroker)GetService(serviceId string) (config.Service, error) {
	for _,s := range sb.config.Services {
		if strings.EqualFold(s.Id, serviceId) {
			return s, nil
		}
	}
	return *new(config.Service), nil
}

func (sb *APMSserviceBroker)GetPlan(serviceId, planId string) (config.Plan, error) {
	for _,s := range sb.config.Services {
		if strings.EqualFold(s.Id, serviceId) {
			for _,p := range s.Plans {
				if strings.EqualFold(p.Id, planId){
					return p, nil
				}
			}
		}
	}
	return *new(config.Plan), nil
}

func servicePlans(plans []config.Plan) []brokerapi.ServicePlan {
	servicePlans := make([]brokerapi.ServicePlan, len(plans)-1)
	for _,servicePlan := range plans {
		servicePlans = append(servicePlans, brokerapi.ServicePlan{
			ID:		servicePlan.Id,
			Name:		servicePlan.Name,
			Description:	servicePlan.Description,
			Free:		servicePlan.Free,
			Bindable:	servicePlan.Bindable,
			Metadata:	&brokerapi.ServicePlanMetadata{
				Bullets: 	servicePlan.Metadata.Bullets,
			},
		})
	}
	return servicePlans
}