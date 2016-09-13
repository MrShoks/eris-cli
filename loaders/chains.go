package loaders

import (
	"fmt"
	"io/ioutil"
	"path"
	"path/filepath"

	"github.com/eris-ltd/eris-cli/config"
	"github.com/eris-ltd/eris-cli/definitions"
	"github.com/eris-ltd/eris-cli/util"
	// "github.com/eris-ltd/eris-cli/version"

	"github.com/eris-ltd/common/go/common"
	log "github.com/eris-ltd/eris-logger"

	"github.com/spf13/viper"
)

// Standard package commands.
// TODO remove (consult w/ ben)
const (
	ErisChainStart = "run"
	ErisChainNew   = "new"
)

// [zr] update this...
// LoadChainConfigFile reads the "default" then chainName definition files
// from the common.ChainsPath directory and returns a chain structure set
// accordingly. LoadChainConfigFile also returns missing files or definition
// reading errors, if any.

func LoadChainConfigFile(chainName string) (*definitions.ChainDefinition, error) {
	chain := definitions.BlankChainDefinition()
	chain.Name = chainName
	chain.Operations.ContainerType = definitions.TypeChain
	chain.Operations.Labels = util.Labels(chain.Name, chain.Operations)

	whereIsTheConfigFile := filepath.Join(common.ChainsPath, chainName, "CONFIG_PATH")
	var err error
	pathToConfig, err := ioutil.ReadFile(whereIsTheConfigFile)
	if err != nil {
		return nil, err
	}

	definition := viper.New()
	definition, err = config.LoadViper(string(pathToConfig), "config")
	if err != nil {
		return nil, err
	}

	// remove ?
	if err = MarshalChainDefinition(definition, chain); err != nil {
		return nil, err
	}

	if chain.Dependencies != nil {
		addDependencyVolumesAndLinks(chain.Dependencies, chain.Service, chain.Operations)
	}

	// check chain names
	chain.Service.Name = chain.Name
	chain.Operations.SrvContainerName = util.ChainContainerName(chain.Name)
	chain.Operations.DataContainerName = util.DataContainerName(chain.Name)

	log.WithFields(log.Fields{
		"chain name":  chain.Name,
		"environment": chain.Service.Environment,
		"entrypoint":  chain.Service.EntryPoint,
		"cmd":         chain.Service.Command,
	}).Debug("Chain definition loaded")
	return chain, nil
}

// ChainsAsAService convert the chain definition to a service one
// and set the CHAIN_ID environment variable. ChainsAsAService
// can return config load errors.
func ChainsAsAService(chainName string) (*definitions.ServiceDefinition, error) {
	chain, err := LoadChainConfigFile(chainName)
	if err != nil {
		return nil, err
	}

	chain.Service.Name = chain.Name
	chain.Service.Image = path.Join(config.Global.DefaultRegistry, config.Global.ImageDB)
	chain.Service.AutoData = true
	chain.Service.Command = ErisChainStart

	s := &definitions.ServiceDefinition{
		Name:         chain.Name,
		ServiceID:    chain.Chain.ChainID, //  hmm
		Dependencies: &definitions.Dependencies{Services: []string{"keys"}},
		Service:      chain.Service,
		Operations:   chain.Operations,
		Maintainer:   chain.Maintainer,
		Location:     chain.Location,
		Machine:      chain.Machine,
	}

	// These are mostly operational considerations that we want to ensure are met.
	ServiceFinalizeLoad(s)

	s.Operations.SrvContainerName = util.ChainContainerName(chainName)
	s.Service.Environment = append(s.Service.Environment, "CHAIN_ID="+chainName) // TODO fix
	return s, nil
}

// ConnectToAChain operates in two ways
//  - if link is true, sets srv.Links to point to a chain container specifiend by name:internalName
//  - if mount is true, sets srv.VolumesFrom to point to a chain container specified by name
func ConnectToAChain(srv *definitions.Service, ops *definitions.Operation, name, internalName string, link, mount bool) {
	connectToAService(srv, ops, definitions.TypeChain, name, internalName, link, mount)
}

// MarshalChainDefinition reads the definition file and sets chain.Service fields
// in the chain structure. Returns config read errors.
func MarshalChainDefinition(definition *viper.Viper, chain *definitions.ChainDefinition) error {
	log.Debug("Marshalling chain")
	chnTemp := definitions.BlankChainDefinition()

	if err := definition.Unmarshal(chnTemp); err != nil {
		return fmt.Errorf("The marmots coult not read the chain definition: %v", err)
	}

	util.Merge(chain.Service, chnTemp.Service)

	if len(chnTemp.Service.Ports) != 0 {
		chain.Service.Ports = chnTemp.Service.Ports
	}

	// toml bools don't really marshal well "data_container". It can be
	// in the chain or in the service layer.
	for _, s := range []string{"", "service."} {
		if definition.GetBool(s + "data_container") {
			chain.Service.AutoData = true
			log.WithField("autodata", chain.Service.AutoData).Debug()
		}
	}

	return nil
}
