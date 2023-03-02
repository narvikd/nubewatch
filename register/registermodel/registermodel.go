package registermodel

import "time"

type Server struct {
	ServiceName         string    `json:"serviceName"`
	ID                  string    `json:"id"`
	Host                string    `json:"host"`
	Protocol            string    `json:"protocol"`
	HealthCheckEndpoint string    `json:"healthCheckEndpoint"`
	CleanFuncName       string    `json:"cleanFuncName"`
	LastContact         time.Time `json:"lastContact"`
	Alive               bool      `json:"alive"`
}
