package crawler

import (
	"context"
	"math/rand/v2"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/modelos/modelos/node/wire"
)

// Address wraps a wire.NetAddress with bookkeeping metadata.
type Address struct {
	netaddr    *wire.NetAddress
	lastUpdate time.Time
}

func (a *Address) String() string {
	return net.JoinHostPort(a.netaddr.IP.String(), strconv.Itoa(int(a.netaddr.Port)))
}

func (a *Address) asPeerKey() PeerKey {
	return PeerKey(a.String())
}

func addressFromPeerKey(s PeerKey) (*Address, error) {
	host, portString, err := net.SplitHostPort(s.String())
	if err != nil {
		return nil, err
	}
	portInt, err := strconv.ParseUint(portString, 10, 16)
	if err != nil {
		return nil, err
	}
	na := wire.NewNetAddressTimestamp(
		time.Now(), 0, net.ParseIP(host), uint16(portInt),
	)
	return &Address{netaddr: na, lastUpdate: na.Timestamp}, nil
}

// AddressBook tracks known-good and blacklisted peer addresses.
type AddressBook struct {
	peers     map[PeerKey]*Address
	blacklist map[PeerKey]*Address

	mu       sync.RWMutex
	notifyCh chan struct{} // closed and replaced on Add/Redeem to wake waiters
}

// NewAddressBook returns an empty AddressBook.
func NewAddressBook() *AddressBook {
	return &AddressBook{
		peers:     make(map[PeerKey]*Address),
		blacklist: make(map[PeerKey]*Address),
		notifyCh:  make(chan struct{}),
	}
}

// Add inserts a peer into the address book.
func (ab *AddressBook) Add(pk PeerKey) {
	addr, err := addressFromPeerKey(pk)
	if err != nil {
		return
	}
	ab.mu.Lock()
	ab.peers[pk] = addr
	ch := ab.notifyCh
	ab.notifyCh = make(chan struct{})
	ab.mu.Unlock()
	close(ch)
}

// Remove deletes a peer from the address book.
func (ab *AddressBook) Remove(pk PeerKey) {
	ab.mu.Lock()
	defer ab.mu.Unlock()
	delete(ab.peers, pk)
}

// Blacklist moves a peer from good to blacklisted, or creates a new
// blacklist entry if the peer wasn't previously known.
func (ab *AddressBook) Blacklist(pk PeerKey) {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	if target, ok := ab.peers[pk]; ok {
		ab.blacklist[pk] = target
		delete(ab.peers, pk)
	} else {
		addr, err := addressFromPeerKey(pk)
		if err != nil {
			return
		}
		ab.blacklist[pk] = addr
	}
}

// Redeem moves a peer from the blacklist back to good.
func (ab *AddressBook) Redeem(pk PeerKey) {
	ab.mu.Lock()
	if addr, ok := ab.blacklist[pk]; ok {
		delete(ab.blacklist, pk)
		addr.lastUpdate = time.Now()
		ab.peers[pk] = addr
	}
	ch := ab.notifyCh
	ab.notifyCh = make(chan struct{})
	ab.mu.Unlock()
	close(ch)
}

// DropFromBlacklist removes a peer from the blacklist entirely.
func (ab *AddressBook) DropFromBlacklist(pk PeerKey) {
	ab.mu.Lock()
	defer ab.mu.Unlock()
	delete(ab.blacklist, pk)
}

// Touch updates the last-seen timestamp for a known-good peer.
func (ab *AddressBook) Touch(pk PeerKey) {
	ab.mu.Lock()
	defer ab.mu.Unlock()
	if target, ok := ab.peers[pk]; ok {
		target.lastUpdate = time.Now()
	}
}

// Count returns the number of known-good peers.
func (ab *AddressBook) Count() int {
	ab.mu.RLock()
	defer ab.mu.RUnlock()
	return len(ab.peers)
}

// IsKnown returns true if the peer is in the good set or the blacklist.
func (ab *AddressBook) IsKnown(pk PeerKey) bool {
	ab.mu.RLock()
	defer ab.mu.RUnlock()
	_, good := ab.peers[pk]
	_, bad := ab.blacklist[pk]
	return good || bad
}

// IsBlacklisted returns true if the peer is blacklisted.
func (ab *AddressBook) IsBlacklisted(pk PeerKey) bool {
	ab.mu.RLock()
	defer ab.mu.RUnlock()
	_, ok := ab.blacklist[pk]
	return ok
}

// EnqueueAddrs returns a closed, buffered channel containing all known-good
// addresses. Consumers can safely range over it.
func (ab *AddressBook) EnqueueAddrs() <-chan *Address {
	ab.mu.RLock()
	defer ab.mu.RUnlock()
	ch := make(chan *Address, len(ab.peers))
	for _, v := range ab.peers {
		ch <- v
	}
	close(ch)
	return ch
}

// EnqueueBlacklist returns a closed, buffered channel containing all
// blacklisted addresses. Consumers can safely range over it.
func (ab *AddressBook) EnqueueBlacklist() <-chan *Address {
	ab.mu.RLock()
	defer ab.mu.RUnlock()
	ch := make(chan *Address, len(ab.blacklist))
	for _, v := range ab.blacklist {
		ch <- v
	}
	close(ch)
	return ch
}

// WaitForAddresses blocks until at least n addresses are known or ctx is
// cancelled. Returns true if the threshold was met, false on cancellation.
func (ab *AddressBook) WaitForAddresses(ctx context.Context, n int) bool {
	for {
		ab.mu.RLock()
		count := len(ab.peers)
		ch := ab.notifyCh
		ab.mu.RUnlock()

		if count >= n {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-ch:
		}
	}
}

// ShuffleAddressList returns up to n IPv4 or IPv6 addresses on the default
// port, in random order. DNS can only return IPs (not ports), so we filter
// to the standard port.
func (ab *AddressBook) ShuffleAddressList(n int, v6 bool, defaultPort string) []net.IP {
	ab.mu.RLock()
	defer ab.mu.RUnlock()

	resp := make([]net.IP, 0, len(ab.peers))
	for pk, addr := range ab.peers {
		if _, blacklisted := ab.blacklist[pk]; blacklisted {
			continue
		}
		ip := addr.netaddr.IP
		isV4 := ip.To4() != nil
		if v6 && isV4 {
			continue
		}
		if !v6 && !isV4 {
			continue
		}
		if strconv.Itoa(int(addr.netaddr.Port)) != defaultPort {
			continue
		}
		resp = append(resp, ip)
	}

	rand.Shuffle(len(resp), func(i, j int) {
		resp[i], resp[j] = resp[j], resp[i]
	})

	if len(resp) > n {
		return resp[:n]
	}
	return resp
}
