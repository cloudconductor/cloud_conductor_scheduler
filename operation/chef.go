package operation

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"scheduler/config"
	"scheduler/util"
	"strings"

	"github.com/hashicorp/consul/api"
	"github.com/imdario/mergo"
)

type ChefOperation struct {
	BaseOperation
	RunList    []string `json:"run_list"`
	Attributes map[string]interface{}
}

type ChefJson struct {
	RunList        []string               `json:"run_list"`
	CloudConductor map[string]interface{} `json:"cloudconductor"`
}

func NewChefOperation(v json.RawMessage) *ChefOperation {
	o := &ChefOperation{}
	json.Unmarshal(v, &o)

	return o
}

func (o *ChefOperation) Run(vars map[string]string) error {
	json, err := o.createJson(util.ParseArray(o.RunList, vars), util.ParseMap(o.Attributes, vars))
	if err != nil {
		return err
	}

	conf, err := o.createConf(vars)
	if err != nil {
		return err
	}

	return o.executeChef(conf, json)
}

func (o *ChefOperation) createJson(runlist []string, overwriteAttributes map[string]interface{}) (string, error) {
	attributes := make(map[string]interface{})
	err := getAttributes(&attributes)
	if err != nil {
		return "", err
	}
	err = mergeAttributes(attributes, overwriteAttributes)
	if err != nil {
		return "", err
	}

	err = getServers(attributes)
	if err != nil {
		return "", err
	}
	json, err := writeJson(runlist, attributes)
	if err != nil {
		return "", err
	}
	return json, nil
}

func getAttributes(out *map[string]interface{}) error {
	var c *api.Client = util.Consul()
	kv, _, err := c.KV().Get("cloudconductor/parameters", &api.QueryOptions{})
	if err != nil {
		return err
	}
	return json.Unmarshal(kv.Value, out)
}

func mergeAttributes(src, dst map[string]interface{}) error {
	patterns := src["cloudconductor"].(map[string]interface{})["patterns"].(map[string]interface{})

	for k, v := range dst {
		m := patterns[k].(map[string]interface{})["user_attributes"].(map[string]interface{})
		err := mergo.MergeWithOverwrite(&m, v)
		if err != nil {
			return errors.New(fmt.Sprintf("Failed to merge attributes(%v)", err))
		}
	}
	return nil
}

func getServers(attributes map[string]interface{}) error {
	var c *api.Client = util.Consul()
	consulServers, _, err := c.KV().List("cloudconductor/servers", &api.QueryOptions{})
	if err != nil {
		return err
	}
	servers := make(map[string]interface{})
	attributes["cloudconductor"].(map[string]interface{})["servers"] = servers
	for _, s := range consulServers {
		node := strings.TrimPrefix(s.Key, "cloudconductor/servers/")
		v := make(map[string]interface{})
		err = json.Unmarshal(s.Value, &v)
		servers[node] = v
		if err != nil {
			return err
		}
	}
	return nil
}

func writeJson(r []string, a map[string]interface{}) (string, error) {
	j := &ChefJson{RunList: r, CloudConductor: a["cloudconductor"].(map[string]interface{})}

	b, err := json.Marshal(j)
	if err != nil {
		return "", err
	}

	f, err := ioutil.TempFile("", "chef-node-json")
	if err != nil {
		return "", err
	}
	defer f.Close()

	_, err = f.Write(b)
	if err != nil {
		return "", err
	}

	return f.Name(), nil
}

func (o *ChefOperation) createConf(vars map[string]string) (string, error) {
	f, err := ioutil.TempFile("", "chef-conf")
	if err != nil {
		return "", err
	}
	defer f.Close()

	m, err := o.defaultConfig()
	if err != nil {
		return "", err
	}

	for k, v := range m {
		if _, ok := vars[k]; ok {
			v = vars[k]
		}
		_, err = f.WriteString(fmt.Sprintf("%s %s\n", k, v))
		if err != nil {
			return "", err
		}
	}

	return f.Name(), nil
}

func (o *ChefOperation) defaultConfig() (map[string]string, error) {
	m := map[string]string{
		"ssl_verify_mode": ":verify_peer",
		"role_path":       "[]",
		"log_level":       ":info",
		"log_location":    "",
		"file_cache_path": "",
		"cookbook_path":   "[]",
	}

	var roleDirs []string
	var cookbookDirs []string

	patternDir := filepath.Join(config.BaseDir, "patterns", o.pattern)

	var dir string
	dir = "'" + filepath.Join(patternDir, "roles") + "'"
	roleDirs = append(roleDirs, dir)

	dir = "'" + filepath.Join(patternDir, "cookbooks") + "'"
	cookbookDirs = append(cookbookDirs, dir)
	dir = "'" + filepath.Join(patternDir, "site-cookbooks") + "'"
	cookbookDirs = append(cookbookDirs, dir)

	m["log_location"] = "'" + filepath.Join(patternDir, "logs", o.pattern+"_chef-solo.log") + "'"
	m["file_cache_path"] = "'" + filepath.Join(patternDir, "tmp", "cache") + "'"
	m["role_path"] = "[" + strings.Join(roleDirs, ", ") + "]"
	m["cookbook_path"] = "[" + strings.Join(cookbookDirs, ", ") + "]"
	return m, nil
}

func (o *ChefOperation) executeChef(conf string, json string) error {
	defer os.Remove(conf)
	defer os.Remove(json)

	fmt.Printf("Execute chef(conf: %s, json: %s)\n", conf, json)
	cmd := exec.Command("chef-solo", "-c", conf, "-j", json)
	cmd.Dir = filepath.Join(config.BaseDir, "patterns", o.pattern)
	return cmd.Run()
}

func (o *ChefOperation) String() string {
	return "chef"
}
