package config

import "time"

type Config struct {
	AllowUserProvisionParameters bool    		`yaml:"allow_user_provision_parameters"`
	AllowUserUpdateParameters    bool    		`yaml:"allow_user_update_parameters"`
	AllowUserBindParameters      bool               `yaml:"allow_user_bind_parameters"`
	DatabaseConfig               DB                 `yaml:"db"`
	Services                     []Service 		`yaml:"services"`
}

type DB struct {
	Endpoints		[]string		`yaml:"endpoints"`
	DbName                  string                  `yaml:"db_name"`
	DialTimeout		time.Duration		`yaml:"dial_timeout"`
}

type Service struct {
	Id          		string 			`yaml:"id"`
	Name        		string 			`yaml:"name"`
	Description 		string 			`yaml:"description"`
	Tags        	 	[]string		`yaml:"tags"`
	Requires    	 	[]string		`yaml:"requires"`
	Bindable    	 	bool			`yaml:"bindable"`
	Metadata    	 	ServiceMetadata		`yaml:"metadata"`
	DashboardClient  	map[string]string	`yaml:"dashboard_client"`
	PlanUpdateable   	bool			`yaml:"plan_updateable"`
	Plans 			[]Plan 			`yaml:"plans"`
}

type ServiceMetadata struct {
	DisplayName         	string 			`yaml:"displayName"`
	ImageUrl            	string 			`yaml:"imageUrl"`
	LongDescription     	string 			`yaml:"longDescription"`
	ProviderDisplayName 	string 			`yaml:"providerDisplayName"`
	DocumentationUrl    	string 			`yaml:"documentationUrl"`
	SupportUrl          	string 			`yaml:"supportUrl"`
}

type Plan struct {
	Id          		string 			`yaml:"id"`
	Name        		string 			`yaml:"name"`
	Description 		string 			`yaml:"description"`
	Free        		*bool 			`yaml:"free"`
	Bindable    		*bool			`yaml:"bindable"`
	Metadata    		PlanMetadata		`yaml:"metadata"`
	Parameters              map[string]interface{} `yaml:"parameters"`
}

type PlanMetadata struct {
	Costs    		[]Cost			`yaml:"costs"`
	Bullets  		[]string		`yaml:"bullets"`
}

type Cost struct {
	Amount    		map[string]float64	`yaml:"amount"`
	Unit      		string			`yaml:"unit"`
}