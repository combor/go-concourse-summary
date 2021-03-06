package summary

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

var defaultRefreshInterval = 30

type indexStruct struct {
	Hosts  []Host
	Groups CSGroups
}

// Config - configuration object for summary
type Config struct {
	RefreshInterval   int
	CSGroups          CSGroups
	Hosts             []Host
	SkipSSLValidation bool
	Templates         *template.Template
	Protocol          string
	Team              string
}

// CSGroups is a collection of concourse summary groups
type CSGroups []CSGroup

// CSGroup is a concourse summary group
type CSGroup struct {
	Group string `json:"group"`
	Hosts []Host `json:"hosts"`
}

// Host is a concourse host defined within a concourse summary group
type Host struct {
	FQDN      string     `json:"fqdn"`
	Pipelines []Pipeline `json:"pipelines"`
}

// Pipeline is a pipeline definted within a concourse summary group host
type Pipeline struct {
	Name   string   `json:"name"`
	Groups []string `json:"groups"`
}

type headerStruct struct {
	RefreshInterval int
}

func (h headerStruct) Now() string {
	return time.Now().Format("2006-01-02 15:04:05 -0700")
}

type hostStruct struct {
	SingleHost singleHostStruct
	Header     headerStruct
}

type groupStruct struct {
	Header headerStruct
	Groups []GroupData
}

type singleHostStruct struct {
	Statuses []Data
}

// SetupConfig sets up a config object for summary, adding default values where appropriate
func SetupConfig(refreshInterval, groupsJSON, hostsJSON, skipSSLValidationString, teamName string) (*Config, error) {
	var (
		refreshIntervalInt int
		err                error
	)

	if refreshInterval == "" {
		refreshIntervalInt = defaultRefreshInterval
	} else {
		refreshIntervalInt, err = strconv.Atoi(refreshInterval)
		if err != nil {
			return &Config{}, err
		}

		if refreshIntervalInt < 1 {
			refreshIntervalInt = defaultRefreshInterval
		}
	}

	var groups CSGroups

	if groupsJSON == "" {
		groupsJSON = "[]"
	}

	if err := json.Unmarshal([]byte(groupsJSON), &groups); err != nil {
		return &Config{}, err
	}

	var hostsSlice []string

	if hostsJSON == "" {
		hostsJSON = "[]"
	}

	if err := json.Unmarshal([]byte(hostsJSON), &hostsSlice); err != nil {
		return &Config{}, err
	}

	var hosts []Host

	for _, host := range hostsSlice {
		hosts = append(hosts, Host{FQDN: host})
	}

	var skipSSLValidation bool
	if skipSSLValidationString == "true" {
		skipSSLValidation = true
	}

	if teamName == "" {
		teamName = "main"
	}

	return &Config{
		RefreshInterval:   refreshIntervalInt,
		CSGroups:          groups,
		Hosts:             hosts,
		SkipSSLValidation: skipSSLValidation,
		Protocol:          "https",
		Team:              teamName,
	}, nil
}

// Index renders and serves the index page
func (config *Config) Index(w http.ResponseWriter, r *http.Request) {
	err := config.Templates.ExecuteTemplate(w, "index", indexStruct{Hosts: config.Hosts, Groups: config.CSGroups})
	if err != nil {
		panic(err.Error())
	}
}

// HostSummary renders and serves the host page
func (config *Config) HostSummary(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	host := vars["host"]
	values, err := getData(host, config)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Error collecting data from concourse (%s) please refer to logs for more details", host)
		fmt.Println(err.Error())
		return
	}

	err = config.Templates.ExecuteTemplate(w, "host", hostStruct{
		Header: headerStruct{
			RefreshInterval: config.RefreshInterval,
		},
		SingleHost: singleHostStruct{
			Statuses: values,
		},
	})
	if err != nil {
		panic(err.Error())
	}
}

// GroupSummary renders and serves the group
func (config *Config) GroupSummary(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	group := vars["group"]
	csGroup := config.CSGroups.group(group)

	var groupsData []GroupData
	for _, host := range csGroup.Hosts {
		values, err := getData(host.FQDN, config)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Error collecting data from concourse (%s) please refer to logs for more details", host.FQDN)
			fmt.Println(err.Error())
			return
		}
		groupsData = append(groupsData, GroupData{Host: host.FQDN, Statuses: filterData(values, host.Pipelines)})
	}

	err := config.Templates.ExecuteTemplate(w, "group", groupStruct{
		Header: headerStruct{
			RefreshInterval: config.RefreshInterval,
		},
		Groups: groupsData,
	})

	if err != nil {
		panic(err.Error())
	}
}

func (csGroups CSGroups) group(group string) CSGroup {
	for _, csGroup := range csGroups {
		if csGroup.Group == group {
			return csGroup
		}
	}
	return CSGroup{}
}
