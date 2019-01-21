package bgp

import (
	"context"
	"os/signal"

	api "github.com/osrg/gobgp/api"
	gobgp "github.com/osrg/gobgp/pkg/server"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"github.com/magicsong/porter/pkg/bgp/config"
	"github.com/osrg/gobgp/internal/pkg/table"

)
var bgpServer *gobgp.BgpServer
func Serve() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)
	log := logf.Log.WithName("gobgpd")
	var opts struct {
		ConfigFile      string `short:"f" long:"config-file" description:"specifying a config file"`
		ConfigType      string `short:"t" long:"config-type" description:"specifying config type (toml, yaml, json)" default:"toml"`
	}
	_, err := flags.Parse(&opts)
	if err != nil {
		os.Exit(1)
	}

	log.Info("gobgpd started")
	bgpServer = gobgp.NewBgpServer()
	go bgpServer.Serve()

	log.Info("read configfile")
	configCh := make(chan *config.BgpConfigSet)
	go config.ReadConfigfileServe(opts.ConfigFile, opts.ConfigType, configCh)

	loop := func() {
		var c *config.BgpConfigSet
		for {
			select {
			case <-sigCh:
				bgpServer.StopBgp(context.Background(), &api.StopBgpRequest{})
				return
			case newConfig := <-configCh:
				var added, deleted, updated []config.Neighbor
				var addedPg, deletedPg, updatedPg []config.PeerGroup
				var updatePolicy bool

				if c == nil {
					c = newConfig
					if err := bgpServer.StartBgp(context.Background(), &api.StartBgpRequest{
						Global: config.NewGlobalFromConfigStruct(&c.Global),
					}); err != nil {
						log.Fatalf("failed to set global config: %s", err)
					}
					p := config.ConfigSetToRoutingPolicy(newConfig)
					rp, err := table.NewAPIRoutingPolicyFromConfigStruct(p)
					if err != nil {
						log.Warn(err)
					} else {
						bgpServer.SetPolicies(context.Background(), &api.SetPoliciesRequest{
							DefinedSets: rp.DefinedSets,
							Policies:    rp.Policies,
						})
					}

					added = newConfig.Neighbors
					addedPg = newConfig.PeerGroups
					if opts.GracefulRestart {
						for i, n := range added {
							if n.GracefulRestart.Config.Enabled {
								added[i].GracefulRestart.State.LocalRestarting = true
							}
						}
					}

				} else {
					addedPg, deletedPg, updatedPg = config.UpdatePeerGroupConfig(c, newConfig)
					added, deleted, updated = config.UpdateNeighborConfig(c, newConfig)
					updatePolicy = config.CheckPolicyDifference(config.ConfigSetToRoutingPolicy(c), config.ConfigSetToRoutingPolicy(newConfig))

					if updatePolicy {
						log.Info("Policy config is updated")
						p := config.ConfigSetToRoutingPolicy(newConfig)
						rp, err := table.NewAPIRoutingPolicyFromConfigStruct(p)
						if err != nil {
							log.Warn(err)
						} else {
							bgpServer.SetPolicies(context.Background(), &api.SetPoliciesRequest{
								DefinedSets: rp.DefinedSets,
								Policies:    rp.Policies,
							})
						}
					}
					// global policy update
					if !newConfig.Global.ApplyPolicy.Config.Equal(&c.Global.ApplyPolicy.Config) {
						a := newConfig.Global.ApplyPolicy.Config
						toDefaultTable := func(r config.DefaultPolicyType) table.RouteType {
							var def table.RouteType
							switch r {
							case config.DEFAULT_POLICY_TYPE_ACCEPT_ROUTE:
								def = table.ROUTE_TYPE_ACCEPT
							case config.DEFAULT_POLICY_TYPE_REJECT_ROUTE:
								def = table.ROUTE_TYPE_REJECT
							}
							return def
						}
						toPolicies := func(r []string) []*table.Policy {
							p := make([]*table.Policy, 0, len(r))
							for _, n := range r {
								p = append(p, &table.Policy{
									Name: n,
								})
							}
							return p
						}

						def := toDefaultTable(a.DefaultImportPolicy)
						ps := toPolicies(a.ImportPolicyList)
						bgpServer.SetPolicyAssignment(context.Background(), &api.SetPolicyAssignmentRequest{
							Assignment: table.NewAPIPolicyAssignmentFromTableStruct(&table.PolicyAssignment{
								Name:     table.GLOBAL_RIB_NAME,
								Type:     table.POLICY_DIRECTION_IMPORT,
								Policies: ps,
								Default:  def,
							}),
						})

						def = toDefaultTable(a.DefaultExportPolicy)
						ps = toPolicies(a.ExportPolicyList)
						bgpServer.SetPolicyAssignment(context.Background(), &api.SetPolicyAssignmentRequest{
							Assignment: table.NewAPIPolicyAssignmentFromTableStruct(&table.PolicyAssignment{
								Name:     table.GLOBAL_RIB_NAME,
								Type:     table.POLICY_DIRECTION_EXPORT,
								Policies: ps,
								Default:  def,
							}),
						})

						updatePolicy = true

					}
					c = newConfig
				}
				for _, pg := range addedPg {
					log.Infof("PeerGroup %s is added", pg.Config.PeerGroupName)
					if err := bgpServer.AddPeerGroup(context.Background(), &api.AddPeerGroupRequest{
						PeerGroup: config.NewPeerGroupFromConfigStruct(&pg),
					}); err != nil {
						log.Warn(err)
					}
				}
				for _, pg := range deletedPg {
					log.Infof("PeerGroup %s is deleted", pg.Config.PeerGroupName)
					if err := bgpServer.DeletePeerGroup(context.Background(), &api.DeletePeerGroupRequest{
						Name: pg.Config.PeerGroupName,
					}); err != nil {
						log.Warn(err)
					}
				}
				for _, pg := range updatedPg {
					log.Infof("PeerGroup %v is updated", pg.State.PeerGroupName)
					if u, err := bgpServer.UpdatePeerGroup(context.Background(), &api.UpdatePeerGroupRequest{
						PeerGroup: config.NewPeerGroupFromConfigStruct(&pg),
					}); err != nil {
						log.Warn(err)
					} else {
						updatePolicy = updatePolicy || u.NeedsSoftResetIn
					}
				}
				for _, pg := range updatedPg {
					log.Infof("PeerGroup %s is updated", pg.Config.PeerGroupName)
					if _, err := bgpServer.UpdatePeerGroup(context.Background(), &api.UpdatePeerGroupRequest{
						PeerGroup: config.NewPeerGroupFromConfigStruct(&pg),
					}); err != nil {
						log.Warn(err)
					}
				}
				for _, dn := range newConfig.DynamicNeighbors {
					log.Infof("Dynamic Neighbor %s is added to PeerGroup %s", dn.Config.Prefix, dn.Config.PeerGroup)
					if err := bgpServer.AddDynamicNeighbor(context.Background(), &api.AddDynamicNeighborRequest{
						DynamicNeighbor: &api.DynamicNeighbor{
							Prefix:    dn.Config.Prefix,
							PeerGroup: dn.Config.PeerGroup,
						},
					}); err != nil {
						log.Warn(err)
					}
				}
				for _, p := range added {
					log.Infof("Peer %v is added", p.State.NeighborAddress)
					if err := bgpServer.AddPeer(context.Background(), &api.AddPeerRequest{
						Peer: config.NewPeerFromConfigStruct(&p),
					}); err != nil {
						log.Warn(err)
					}
				}
				for _, p := range deleted {
					log.Infof("Peer %v is deleted", p.State.NeighborAddress)
					if err := bgpServer.DeletePeer(context.Background(), &api.DeletePeerRequest{
						Address: p.State.NeighborAddress,
					}); err != nil {
						log.Warn(err)
					}
				}
				for _, p := range updated {
					log.Infof("Peer %v is updated", p.State.NeighborAddress)
					if u, err := bgpServer.UpdatePeer(context.Background(), &api.UpdatePeerRequest{
						Peer: config.NewPeerFromConfigStruct(&p),
					}); err != nil {
						log.Warn(err)
					} else {
						updatePolicy = updatePolicy || u.NeedsSoftResetIn
					}
				}

				if updatePolicy {
					if err := bgpServer.ResetPeer(context.Background(), &api.ResetPeerRequest{
						Address:   "",
						Direction: api.ResetPeerRequest_IN,
						Soft:      true,
					}); err != nil {
						log.Warn(err)
					}
				}
			}
		}
	}

	loop()
}
