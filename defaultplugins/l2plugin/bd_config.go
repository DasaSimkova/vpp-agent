//go:generate protoc --proto_path=model/l2 --gogo_out=model/l2 model/l2/l2.proto

//go:generate binapi-generator --input-file=/usr/share/vpp/api/l2.api.json --output-dir=bin_api
//go:generate binapi-generator --input-file=/usr/share/vpp/api/vpe.api.json --output-dir=bin_api

// Package l2plugin is the implementation of the L2 plugin that handles BD / L2 FIB.
package l2plugin

import (
	govppapi "git.fd.io/govpp.git/api"
	log "github.com/ligato/cn-infra/logging/logrus"
	"github.com/ligato/vpp-agent/govppmux"
	"github.com/ligato/vpp-agent/idxvpp"
	"github.com/ligato/vpp-agent/defaultplugins/ifplugin/ifaceidx"
	"github.com/ligato/vpp-agent/defaultplugins/l2plugin/bdidx"
	l2ba "github.com/ligato/vpp-agent/defaultplugins/l2plugin/bin_api/l2"
	"github.com/ligato/vpp-agent/defaultplugins/l2plugin/model/l2"
	"github.com/ligato/vpp-agent/defaultplugins/l2plugin/vppcalls"
	"github.com/ligato/cn-infra/utils/safeclose"
)

// BDConfigurator runs in the background in its own goroutine where it watches for any changes
// in the configuration of bridge domains as modelled by the proto file "../model/l2/l2.proto" and stored
// in ETCD under the key "/vnf-agent/{vnf-agent}/vpp/config/v1bd". Updates received from the northbound API
// are compared with the VPP run-time configuration and differences are applied through the VPP binary API.
type BDConfigurator struct {
	BdIndexes     bdidx.BDIndexRW // bridge domains
	IfToBdIndexes idxvpp.NameToIdxRW // interface to bridge domain mapping - desired state. Metadata is boolean flag whether interface is bvi or not
	//TODO use rather BdIndexes.LookupNameByIfaceName
	IfToBdRealStateIdx     idxvpp.NameToIdxRW // interface to bridge domain mapping - current state. Metadata is boolean flag whether interface is bvi or not
	BridgeDomainIDSeq      uint32
	RegisteredIfaceCounter uint32
	vppChan                *govppapi.Channel
	SwIfIndexes            ifaceidx.SwIfIndex

	notificationChan chan govppapi.Message
}

// BridgeDomainMeta holds info about interfaces's bridge domain index and BVI
type BridgeDomainMeta struct {
	BridgeDomainIndex uint32
	IsInterfaceBvi    bool
}

// Init members (channels...) and start go routines.
func (plugin *BDConfigurator) Init(notificationChannel chan govppapi.Message) (err error) {

	log.Debug("Initializing L2 Bridge domains")

	// Init VPP API channel
	plugin.vppChan, err = govppmux.NewAPIChannel()
	if err != nil {
		return err
	}

	// Init notification channel
	plugin.notificationChan = notificationChannel

	err = vppcalls.CheckMsgCompatibilityForBridgeDomains(plugin.vppChan)
	if err != nil {
		return err
	}

	return nil
}

// Close GOVPP channel
func (plugin *BDConfigurator) Close() error {
	return safeclose.Close(plugin.vppChan)
}

// ConfigureBridgeDomain for newly created bridge domain
func (plugin *BDConfigurator) ConfigureBridgeDomain(bridgeDomainInput *l2.BridgeDomains_BridgeDomain) error {
	log.Println("Configuring VPP Bridge Domain", bridgeDomainInput.Name)

	if !plugin.vppValidateBridgeDomainBVI(bridgeDomainInput) {
		return nil
	}

	bridgeDomainIndex := plugin.BridgeDomainIDSeq

	// Create bridge domain with respective index
	err := vppcalls.VppAddBridgeDomain(bridgeDomainIndex, bridgeDomainInput, plugin.vppChan)
	// Increment global index
	plugin.BridgeDomainIDSeq++
	if err != nil {
		log.WithField("Bridge domain name", bridgeDomainInput.Name).Error(err)
		return err
	}

	// Register created bridge domain
	plugin.BdIndexes.RegisterName(bridgeDomainInput.Name, bridgeDomainIndex, nil)
	log.WithFields(log.Fields{"Name": bridgeDomainInput.Name, "Index": bridgeDomainIndex}).Debug("Bridge domain registered.")

	errLookup := plugin.LookupBridgeDomains()
	if errLookup != nil {
		log.WithField("bdName", bridgeDomainInput.Name).Error(errLookup)
		return errLookup
	}

	// Find all interfaces belonging to this bridge domain and set them up
	allInterfaces, configuredInterfaces, bviInterfaceName := vppcalls.VppSetAllInterfacesToBridgeDomain(bridgeDomainInput, bridgeDomainIndex,
		plugin.SwIfIndexes, plugin.vppChan)
	plugin.registerInterfaceToBridgeDomainPairs(allInterfaces, configuredInterfaces, bviInterfaceName, bridgeDomainIndex)

	// Resolve ARP termination table entries
	arpTerminationTable := bridgeDomainInput.GetArpTerminationTable()
	if arpTerminationTable != nil && len(arpTerminationTable) != 0 {
		arpTable := bridgeDomainInput.ArpTerminationTable
		for _, arpEntry := range arpTable {
			err := vppcalls.VppAddArpTerminationTableEntry(bridgeDomainIndex, arpEntry.PhysAddress, arpEntry.IpAddress, plugin.vppChan)
			if err != nil {
				log.Error(err)
			}
		}
	} else {
		log.WithField("Bridge domain name", bridgeDomainInput.Name).Debug("No ARP termination entries to set")
	}

	return nil
}

// ModifyBridgeDomain process the NB config and propagates it to bin api calls
func (plugin *BDConfigurator) ModifyBridgeDomain(newConfig *l2.BridgeDomains_BridgeDomain, oldConfig *l2.BridgeDomains_BridgeDomain) error {
	log.Println("Modifying VPP bridge domain", newConfig.Name)

	// Validate config
	if !plugin.vppValidateBridgeDomainBVI(newConfig) {
		return nil
	}

	oldConfigIndex, _, found := plugin.BdIndexes.LookupIdx(oldConfig.Name)
	// During update, old bridge domain will be removed (if exists), so unregister all interfaces at first
	if found {
		oldInterfaces := vppcalls.VppUnsetAllInterfacesFromBridgeDomain(oldConfig, oldConfigIndex,
			plugin.SwIfIndexes, plugin.vppChan)
		plugin.unregisterInterfaceToBridgeDomainPairs(oldInterfaces)
	}

	// In case new bridge domain does not exist, create it. But is shouldn't happen
	newConfigIndex, _, found := plugin.BdIndexes.LookupIdx(newConfig.Name)
	if !found {
		err := plugin.ConfigureBridgeDomain(newConfig)
		if err != nil {
			return err
		}
	}

	// Refresh bridge domain params. Old bridge domain will removed if exists
	err := vppcalls.VppUpdateBridgeDomain(oldConfigIndex, newConfigIndex, newConfig, plugin.vppChan)
	if err != nil {
		log.WithField("Bridge domain name", newConfig.Name).Error(err)
		return err
	}
	log.WithField("Bridge domain name", newConfig.Name).Debug("Bridge domain params updated.")

	// Reload interfaces for new modified bridge domain, remove any out-of-date interface to BD pairs and register new ones if necessary
	allNewInterfaces, configuredNewInterfaces, bvi := vppcalls.VppSetAllInterfacesToBridgeDomain(newConfig,
		newConfigIndex, plugin.SwIfIndexes, plugin.vppChan)
	plugin.registerInterfaceToBridgeDomainPairs(allNewInterfaces, configuredNewInterfaces, bvi, newConfigIndex)

	// Update ARP termination
	if len(newConfig.ArpTerminationTable) == 0 {
		log.Debug("No new entries to arp termination table")
	} else if len(oldConfig.ArpTerminationTable) == 0 && len(newConfig.ArpTerminationTable) != 0 {
		arpTable := newConfig.GetArpTerminationTable()
		for _, entry := range arpTable {
			vppcalls.VppAddArpTerminationTableEntry(newConfigIndex, entry.PhysAddress, entry.IpAddress, plugin.vppChan)
		}
	} else if len(oldConfig.ArpTerminationTable) != 0 {
		odlArpTable := oldConfig.GetArpTerminationTable()
		newArpTable := newConfig.GetArpTerminationTable()
		// in case old BD was not removed, delete old apr entries
		oldBdIndex, _, found := plugin.BdIndexes.LookupIdx(oldConfig.Name)
		if found {
			for _, entry := range odlArpTable {
				vppcalls.VppRemoveArpTerminationTableEntry(oldBdIndex, entry.PhysAddress, entry.IpAddress, plugin.vppChan)
			}
		}
		for _, entry := range newArpTable {
			vppcalls.VppAddArpTerminationTableEntry(newConfigIndex, entry.PhysAddress, entry.IpAddress, plugin.vppChan)
		}
	}

	return nil
}

// DeleteBridgeDomain  process the NB config and propagates it to bin api calls
func (plugin *BDConfigurator) DeleteBridgeDomain(bridgeDomain *l2.BridgeDomains_BridgeDomain) error {
	log.Println("'Deleting' bridge domain", bridgeDomain.Name)

	bdIdx, _, found := plugin.BdIndexes.LookupIdx(bridgeDomain.Name)
	if !found {
		log.WithField("bdName", bridgeDomain.Name).Debug("Unable to find index for bridge domain to be deleted.")
		return nil
	}

	return plugin.deleteBridgeDomain(bridgeDomain, bdIdx)
}

func (plugin *BDConfigurator) deleteBridgeDomain(bridgeDomain *l2.BridgeDomains_BridgeDomain, bdIdx uint32) error {
	// Unmap all interfaces from removed bridge domain
	interfaces := vppcalls.VppUnsetAllInterfacesFromBridgeDomain(bridgeDomain, bdIdx,
		plugin.SwIfIndexes, plugin.vppChan)
	plugin.unregisterInterfaceToBridgeDomainPairs(interfaces)

	err := vppcalls.VppDeleteBridgeDomain(bdIdx, plugin.vppChan)
	if err != nil {
		return err
	}

	plugin.BdIndexes.UnregisterName(bridgeDomain.Name)
	log.WithFields(log.Fields{"Name": bridgeDomain.Name, "bdIdx": bdIdx}).Debug("Bridge domain removed.")

	return nil
}

// LookupBridgeDomains looks up all VPP BDs and saves their name-to-index mapping
func (plugin *BDConfigurator) LookupBridgeDomains() error {
	req := &l2ba.BridgeDomainDump{}
	reqContext := plugin.vppChan.SendMultiRequest(req)
	for {
		msg := &l2ba.BridgeDomainDetails{}
		stop, err := reqContext.ReceiveReply(msg)
		if err != nil {
			log.Error(err)
			return err
		}
		if stop {
			break
		}
		// Store name if missing
		_, meta, found := plugin.BdIndexes.LookupName(msg.BdID)
		bdName := string(msg.BdID)
		if !found {
			log.WithFields(log.Fields{"Name": bdName, "Index": msg.BdID}).Debug("Bridge domain registered.")
			plugin.BdIndexes.RegisterName(bdName, msg.BdID, meta)
		} else {
			log.WithFields(log.Fields{"Name": bdName, "Index": msg.BdID}).Debug("Bridge domain already registered.")
		}
	}

	return nil
}

// ResolveCreatedInterface looks for bridge domain this interface is assigned to and sets it up
func (plugin *BDConfigurator) ResolveCreatedInterface(interfaceName string, interfaceIndex uint32) {
	log.Println("Resolving new interface ", interfaceName)
	// Look whether interface belongs to some bridge domain using interface-to-bd mapping
	_, meta, found := plugin.IfToBdIndexes.LookupIdx(interfaceName)
	if !found {
		log.Debug("Interface does not belong to any bridge domain ", interfaceName)
		return
	}

	bridgeDomainIndex := meta.(*BridgeDomainMeta).BridgeDomainIndex
	bvi := meta.(*BridgeDomainMeta).IsInterfaceBvi

	vppcalls.VppSetInterfaceToBridgeDomain(bridgeDomainIndex, interfaceIndex, bvi, plugin.vppChan)
	// Register interface to real state
	plugin.IfToBdRealStateIdx.RegisterName(interfaceName, interfaceIndex, meta)
}

// ResolveDeletedInterface does nothing
func (plugin *BDConfigurator) ResolveDeletedInterface(interfaceName string) {
	log.Print("Interface was removed. Unregister from real state ", interfaceName)
	// Unregister removed interface
	plugin.IfToBdRealStateIdx.UnregisterName(interfaceName)
	// Nothing else to do here, vpp handles it itself
}

// Store all interface/bridge domain pairs
func (plugin *BDConfigurator) registerInterfaceToBridgeDomainPairs(allInterfaces []string, configuredInterfaces []string, bviIface string, domainID uint32) {
	if len(allInterfaces) == 0 {
		return
	}
	for _, iface := range allInterfaces {
		bvi := false
		if iface == bviIface {
			bvi = true
		}
		// Prepare metadata
		meta := BridgeDomainMeta{
			BridgeDomainIndex: domainID,
			IsInterfaceBvi:    bvi,
		}
		plugin.IfToBdIndexes.RegisterName(iface, plugin.RegisteredIfaceCounter, &meta)
		log.Debugf("Iface %v to BD %v pair registered", iface, domainID)

		// Find whether interface is configured
		ok := false
		for _, configuredIface := range configuredInterfaces {
			if configuredIface == iface {
				ok = true
				break
			}
		}
		if ok {
			log.Debugf("Iface %v to BD %v pair configured", iface, domainID)
			plugin.IfToBdRealStateIdx.RegisterName(iface, plugin.RegisteredIfaceCounter, &meta)
		}
		plugin.RegisteredIfaceCounter++
	}
}

// Remove all interface/bridge domain pairs from database
func (plugin *BDConfigurator) unregisterInterfaceToBridgeDomainPairs(interfaces []string) {
	if len(interfaces) == 0 {
		return
	}
	// Unregister from desired and current state
	for _, iface := range interfaces {
		plugin.IfToBdIndexes.UnregisterName(iface)
		plugin.IfToBdRealStateIdx.UnregisterName(iface)
		log.WithFields(log.Fields{"Iface": iface}).Debug("Interface to bridge domain unregistered.")
	}
}

func (plugin *BDConfigurator) vppValidateBridgeDomainBVI(bridgeDomain *l2.BridgeDomains_BridgeDomain) bool {
	if len(bridgeDomain.Interfaces) == 0 {
		log.Warnf("Bridge domain %v does not contain any interface", bridgeDomain.Name)
		return true
	}
	var bviCount int
	for _, bdInterface := range bridgeDomain.Interfaces {
		if bdInterface.BridgedVirtualInterface {
			bviCount++
		}
	}
	if bviCount == 0 {
		log.Debugf("Bridge domain %v does not contain any bvi interface", bridgeDomain.Name)
		return true
	} else if bviCount == 1 {
		return true
	} else {
		log.Warnf("Bridge domain %v contains more than one BVI interface. Correct it and create/modify bridge domain again", bridgeDomain.Name)
		return false
	}
}
