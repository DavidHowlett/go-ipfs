package corerouting

import (
	"errors"
	"fmt"

	context "github.com/jbenet/go-ipfs/Godeps/_workspace/src/code.google.com/p/go.net/context"
	datastore "github.com/jbenet/go-ipfs/Godeps/_workspace/src/github.com/jbenet/go-datastore"
	ma "github.com/jbenet/go-ipfs/Godeps/_workspace/src/github.com/jbenet/go-multiaddr"
	core "github.com/jbenet/go-ipfs/core"
	"github.com/jbenet/go-ipfs/p2p/peer"
	routing "github.com/jbenet/go-ipfs/routing"
	grandcentral "github.com/jbenet/go-ipfs/routing/grandcentral"
	gcproxy "github.com/jbenet/go-ipfs/routing/grandcentral/proxy"
	ipfsaddr "github.com/jbenet/go-ipfs/util/ipfsaddr"
)

// NB: DHT option is included in the core to avoid 1) because it's a sane
// default and 2) to avoid a circular dependency (it needs to be referenced in
// the core if it's going to be the default)

var (
	errHostMissing      = errors.New("grandcentral client requires a Host component")
	errIdentityMissing  = errors.New("grandcentral server requires a peer ID identity")
	errPeerstoreMissing = errors.New("grandcentral server requires a peerstore")
	errServersMissing   = errors.New("grandcentral client requires at least 1 server peer")
)

// GrandCentralServer returns a configuration for a routing server that stores
// routing records to the provided datastore. Only routing records are store in
// the datastore.
func GrandCentralServer(recordSource datastore.ThreadSafeDatastore) core.RoutingOption {
	return func(ctx context.Context, node *core.IpfsNode) (routing.IpfsRouting, error) {
		if node.Peerstore == nil {
			return nil, errPeerstoreMissing
		}
		if node.PeerHost == nil {
			return nil, errHostMissing
		}
		if node.Identity == "" {
			return nil, errIdentityMissing
		}
		server, err := grandcentral.NewServer(recordSource, node.Peerstore, node.Identity)
		if err != nil {
			return nil, err
		}
		proxy := &gcproxy.Loopback{
			Handler: server,
			Local:   node.Identity,
		}
		node.PeerHost.SetStreamHandler(gcproxy.ProtocolGCR, proxy.HandleStream)
		return grandcentral.NewClient(proxy, node.PeerHost, node.Peerstore, node.Identity)
	}
}

// TODO doc
func GrandCentralClient(remotes ...ipfsaddr.IPFSAddr) core.RoutingOption {
	return func(ctx context.Context, node *core.IpfsNode) (routing.IpfsRouting, error) {
		if len(remotes) < 1 {
			return nil, errServersMissing
		}
		if node.PeerHost == nil {
			return nil, errHostMissing
		}
		if node.Identity == "" {
			return nil, errIdentityMissing
		}
		if node.Peerstore == nil {
			return nil, errors.New("need peerstore")
		}

		var remoteInfos []peer.PeerInfo
		for _, remote := range remotes {
			remoteInfos = append(remoteInfos, peer.PeerInfo{
				ID:    remote.ID(),
				Addrs: []ma.Multiaddr{},
			})
		}

		// TODO move to bootstrap method
		for _, info := range remoteInfos {
			if err := node.PeerHost.Connect(ctx, info); err != nil {
				return nil, fmt.Errorf("failed to dial %s: %s", info.ID, err)
			}
		}

		// TODO right now, I think this has a hidden dependency on the
		// bootstrap peers provided to the core.Node. Careful...

		var ids []peer.ID
		for _, info := range remoteInfos {
			ids = append(ids, info.ID)
		}
		proxy := gcproxy.Standard(node.PeerHost, ids)
		node.PeerHost.SetStreamHandler(gcproxy.ProtocolGCR, proxy.HandleStream)
		return grandcentral.NewClient(proxy, node.PeerHost, node.Peerstore, node.Identity)
	}
}