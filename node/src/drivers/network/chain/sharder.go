package chain

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"hash"
	"math/rand"
	"sort"
	"strconv"
	"sync"
	// "time"
)

// Node represents a physical or virtual machine in the network.
type Node struct {
	ID string
}

// DApp represents a decentralized application with a specific computational load.
// Its load is now dynamic, measured by the number of transactions it processes.
type DApp struct {
	ID                    string
	mu                    sync.Mutex
	ProcessedTransactions int // Simulates the computational load
}

// ProcessTransaction simulates a new transaction being processed by the DApp.
func (d *DApp) ProcessTransaction() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ProcessedTransactions++
}

// Shard represents a logical partition of the distributed ledger.
// It is maintained by a set of Nodes for redundancy and hosts a collection of DApps.
type Shard struct {
	ID      string
	mu      sync.RWMutex
	Dapps   map[string]*DApp // Maps DApp ID to the DApp
	nodeIDs []string         // The Nodes responsible for this shard
}

// NewShard creates and initializes a new Shard.
func NewShard(id string, nodeIDs []string) *Shard {
	return &Shard{
		ID:      id,
		Dapps:   make(map[string]*DApp),
		nodeIDs: nodeIDs,
	}
}

// AddDApp assigns a new DApp to the shard's state.
func (s *Shard) AddDApp(dapp *DApp) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Dapps[dapp.ID] = dapp
	fmt.Printf("[Shard %s] Added DApp: %s\n", s.ID, dapp.ID)
}

// RemoveDApp removes a DApp from the shard's state.
func (s *Shard) RemoveDApp(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.Dapps, id)
	fmt.Printf("[Shard %s] Removed DApp: %s\n", s.ID, id)
}

// GetDApp retrieves a DApp from the shard's state.
func (s *Shard) GetDApp(id string) (*DApp, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dapp, ok := s.Dapps[id]
	return dapp, ok
}

// DappsDump returns a copy of the shard's DApp collection.
func (s *Shard) DappsDump() []*DApp {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dump := make([]*DApp, 0, len(s.Dapps))
	for _, v := range s.Dapps {
		dump = append(dump, v)
	}
	return dump
}

// ConsistentHasher is the core component for dynamic sharding.
// It maps keys (DApp IDs) to Shards, minimizing rebalancing when Shards are added or removed.
type ConsistentHasher struct {
	hash     hash.Hash
	replicas int
	mutex    sync.RWMutex
	Keys     []int          // Sorted hash ring
	Shards   map[int]string // Map from hash key to shard ID
}

// NewConsistentHasher creates a new ConsistentHasher.
func NewConsistentHasher(replicas int) *ConsistentHasher {
	return &ConsistentHasher{
		hash:     sha1.New(),
		replicas: replicas,
		Shards:   make(map[int]string),
	}
}

// AddShard adds a new shard to the hash ring. It creates multiple virtual Nodes for each shard
// to ensure a more even distribution.
func (ch *ConsistentHasher) AddShard(id string) {
	ch.mutex.Lock()
	defer ch.mutex.Unlock()

	for i := 0; i < ch.replicas; i++ {
		hashKey := ch.hashKey(id + strconv.Itoa(i))
		ch.Keys = append(ch.Keys, hashKey)
		ch.Shards[hashKey] = id
	}
	sort.Ints(ch.Keys)
	fmt.Printf("ConsistentHasher: Added shard %s. Total keys on ring: %d\n", id, len(ch.Keys))
}

// RemoveShard removes a shard from the hash ring and all its virtual Nodes.
func (ch *ConsistentHasher) RemoveShard(id string) {
	ch.mutex.Lock()
	defer ch.mutex.Unlock()

	var newKeys []int
	for _, k := range ch.Keys {
		if ch.Shards[k] != id {
			newKeys = append(newKeys, k)
		} else {
			delete(ch.Shards, k)
		}
	}
	ch.Keys = newKeys
	sort.Ints(ch.Keys)
	fmt.Printf("ConsistentHasher: Removed shard %s. Total keys on ring: %d\n", id, len(ch.Keys))
}

// hashKey generates a SHA1 hash of a string and converts it to an integer.
func (ch *ConsistentHasher) hashKey(key string) int {
	ch.hash.Reset()
	ch.hash.Write([]byte(key))
	return int(ch.hash.Sum(nil)[0])
}

// GetShard determines which shard a key belongs to.
func (ch *ConsistentHasher) GetShard(key string) string {
	ch.mutex.RLock()
	defer ch.mutex.RUnlock()

	if len(ch.Keys) == 0 {
		return ""
	}

	hashKey := ch.hashKey(key)

	// Binary search to find the shard on the ring.
	i := sort.Search(len(ch.Keys), func(i int) bool { return ch.Keys[i] >= hashKey })

	// If no key is found, wrap around to the beginning of the ring.
	if i == len(ch.Keys) {
		i = 0
	}
	return ch.Shards[ch.Keys[i]]
}

// ShardManager orchestrates the distributed ledger network.
// It manages the collection of Nodes and Shards.
type ShardManager struct {
	Nodes  map[string]*Node
	Shards map[string]*Shard
	Dapps  map[string]*DApp // Central registry of all DApps
	Hasher *ConsistentHasher
	mu     sync.RWMutex

	// Parameters for automatic management
	MaxShardLoad int
	MinShardLoad int
	MaxNodes     int
	MinNodes     int
	ShardCounter int64 // Used to generate unique IDs for new Shards
	NodeCounter  int   // Used to generate unique IDs for new Nodes

	createChainCb func(string, []string)
}

// NewShardManager initializes a new network with an initial number of Nodes and Shards.
func NewShardManager(initialNodes []string, initialShards int64, MaxShardLoad, MinShardLoad, MaxNodes, MinNodes int, createChainCallback func(string, []string)) *ShardManager {

	// rand.Seed(time.Now().UnixNano())

	manager := &ShardManager{
		Nodes:         make(map[string]*Node),
		Shards:        make(map[string]*Shard),
		Dapps:         make(map[string]*DApp),
		Hasher:        NewConsistentHasher(20),
		MaxShardLoad:  MaxShardLoad,
		MinShardLoad:  MinShardLoad,
		MaxNodes:      MaxNodes,
		MinNodes:      MinNodes,
		ShardCounter:  initialShards,
		NodeCounter:   len(initialNodes),
		createChainCb: createChainCallback,
	}

	fmt.Println("Initializing distributed ledger network...")

	for _, nodeID := range initialNodes {
		manager.AddNode(nodeID)
	}

	for i := int64(0); i < initialShards; i++ {
		shardID := fmt.Sprintf("shard-%d", i+1)
		manager.AddShard(shardID)
	}

	return manager
}

// ExportState serializes the entire ShardManager state into a JSON string.
func (sm *ShardManager) ExportState() (string, error) {

	data, err := json.MarshalIndent(sm, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to export state: %w", err)
	}
	return string(data), nil
}

// ImportState deserializes a JSON string to rebuild the ShardManager state.
func (sm *ShardManager) ImportState(data string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	var importedManager ShardManager
	err := json.Unmarshal([]byte(data), &importedManager)
	if err != nil {
		return fmt.Errorf("failed to import state: %w", err)
	}

	// Manually re-initialize fields that weren't marshaled.
	sm.Nodes = importedManager.Nodes
	sm.Shards = importedManager.Shards
	sm.Dapps = importedManager.Dapps
	sm.Hasher = NewConsistentHasher(20) // Re-create the hasher
	sm.Hasher.Keys = importedManager.Hasher.Keys
	sm.Hasher.Shards = importedManager.Hasher.Shards
	sm.MaxShardLoad = importedManager.MaxShardLoad
	sm.MinShardLoad = importedManager.MinShardLoad
	sm.MaxNodes = importedManager.MaxNodes
	sm.MinNodes = importedManager.MinNodes
	sm.ShardCounter = importedManager.ShardCounter
	sm.NodeCounter = importedManager.NodeCounter

	fmt.Println("\n--- Successfully imported network state! ---")
	return nil
}

// AddNode adds a new node to the network and assigns it to a random shard.
func (sm *ShardManager) AddNode(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.Nodes[id]; exists {
		fmt.Printf("Node %s already exists.\n", id)
		return
	}

	sm.Nodes[id] = &Node{ID: id}

	if len(sm.Shards) > 0 {
		var shardIDs []string
		for sid := range sm.Shards {
			shardIDs = append(shardIDs, sid)
		}
		targetShardID := shardIDs[rand.Intn(len(shardIDs))]
		sm.Shards[targetShardID].nodeIDs = append(sm.Shards[targetShardID].nodeIDs, id)
		fmt.Printf("Added Node %s and assigned it to shard %s\n", id, targetShardID)
	} else {
		fmt.Printf("Added Node %s, but no Shards exist to assign it to.\n", id)
	}
}

// RemoveNode simulates a node leaving the network and reassigns its Shards.
func (sm *ShardManager) RemoveNode(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.Nodes[id]; !exists {
		fmt.Printf("Node %s not found.\n", id)
		return
	}

	fmt.Printf("\n--- Node %s disconnected. Re-assigning its Shards... ---\n", id)

	for sid, shard := range sm.Shards {
		var newNodes []string
		for _, nid := range shard.nodeIDs {
			if nid != id {
				newNodes = append(newNodes, nid)
			}
		}
		sm.Shards[sid].nodeIDs = newNodes
	}

	delete(sm.Nodes, id)
}

func (sm *ShardManager) manageShards() {
	sm.mu.RLock()
	var shardIDs []string
	for id := range sm.Shards {
		shardIDs = append(shardIDs, id)
	}
	sm.mu.RUnlock()

	for _, id := range shardIDs {
		sm.mu.RLock()
		shard, ok := sm.Shards[id]
		sm.mu.RUnlock()
		if !ok {
			continue
		}

		shard.mu.RLock()
		totalLoad := 0
		for _, dapp := range shard.Dapps {
			dapp.mu.Lock()
			totalLoad += dapp.ProcessedTransactions
			dapp.mu.Unlock()
		}
		shard.mu.RUnlock()

		if totalLoad > sm.MaxShardLoad && len(sm.Shards) < sm.MaxNodes {
			sm.SplitShard(id)
		} else if totalLoad < sm.MinShardLoad && len(sm.Shards) > sm.MinNodes {
			sm.MergeShard(id)
		}
	}
}

// AddShard adds a new logical shard to the network.
func (sm *ShardManager) AddShard(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.Shards[id]; exists {
		fmt.Printf("Shard %s already exists.\n", id)
		return
	}

	var activeNodeIDs []string
	for nid := range sm.Nodes {
		activeNodeIDs = append(activeNodeIDs, nid)
	}

	newShard := NewShard(id, activeNodeIDs)
	sm.Shards[id] = newShard
	sm.Hasher.AddShard(id)
	fmt.Printf("Added logical shard %s and assigned it to Nodes: %v\n", id, newShard.nodeIDs)

	sm.createChainCb(id, newShard.nodeIDs)
}

// MergeShard simulates a shard being merged into others.
func (sm *ShardManager) MergeShard(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	shardToMerge, exists := sm.Shards[id]
	if !exists {
		fmt.Printf("Shard %s not found.\n", id)
		return
	}

	fmt.Printf("\n--- Merging Shard %s (underutilized) ---\n", id)

	DappsToMigrate := shardToMerge.DappsDump()
	sm.Hasher.RemoveShard(id)
	delete(sm.Shards, id)

	for _, dapp := range DappsToMigrate {
		newShardID := sm.Hasher.GetShard(dapp.ID)
		fmt.Printf("Migrating DApp '%s' to new shard '%s'\n", dapp.ID, newShardID)
		if newShard, ok := sm.Shards[newShardID]; ok {
			newShard.AddDApp(dapp)
		}
	}
}

// SplitShard simulates an overloaded shard splitting its data.
func (sm *ShardManager) SplitShard(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	shardToSplit, exists := sm.Shards[id]
	if !exists {
		fmt.Printf("Shard %s not found.\n", id)
		return
	}

	sm.ShardCounter++
	newShardID := fmt.Sprintf("shard-%d", sm.ShardCounter)

	fmt.Printf("\n--- Splitting Shard %s to create new shard %s ---\n", id, newShardID)

	sm.AddShard(newShardID)

	DappsToMigrate := shardToSplit.DappsDump()

	// Sort DApps by their load to ensure a more even split
	sort.Slice(DappsToMigrate, func(i, j int) bool {
		return DappsToMigrate[i].ProcessedTransactions > DappsToMigrate[j].ProcessedTransactions
	})

	halfLoad := 0
	for _, dapp := range DappsToMigrate {
		halfLoad += dapp.ProcessedTransactions
	}
	halfLoad /= 2

	var migratedLoad int
	for _, dapp := range DappsToMigrate {
		if migratedLoad < halfLoad {
			sm.mu.RLock()
			newShard, ok := sm.Shards[newShardID]
			sm.mu.RUnlock()
			if ok {
				newShard.AddDApp(dapp)
				migratedLoad += dapp.ProcessedTransactions
				shardToSplit.RemoveDApp(dapp.ID)
			}
		}
	}
}

// DeployDapp routes a new DApp to the correct shard and deploys it.
func (sm *ShardManager) DeployDapp(dappID string) {
	sm.mu.Lock()
	dapp := &DApp{ID: dappID}
	sm.Dapps[dappID] = dapp
	sm.mu.Unlock()

	shardID := sm.Hasher.GetShard(dappID)
	sm.mu.RLock()
	shard, ok := sm.Shards[shardID]
	sm.mu.RUnlock()

	if !ok {
		fmt.Printf("Error: Shard %s not found for DApp %s. Re-routing...\n", shardID, dappID)
		sm.AddShard(shardID)
		sm.mu.RLock()
		shard = sm.Shards[shardID]
		sm.mu.RUnlock()
	}

	nodeID := shard.nodeIDs[rand.Intn(len(shard.nodeIDs))]
	fmt.Printf("Routing DApp '%s' to node %s on shard %s\n", dappID, nodeID, shard.ID)
	shard.AddDApp(dapp)
	sm.manageShards()
}

// ProcessDAppTransaction simulates a transaction for a given DApp.
func (sm *ShardManager) ProcessDAppTransaction(dappID string) {
	sm.mu.RLock()
	dapp, ok := sm.Dapps[dappID]
	sm.mu.RUnlock()
	if !ok {
		fmt.Printf("Error: DApp %s not found.\n", dappID)
		return
	}
	dapp.ProcessTransaction()
	sm.manageShards()
}

// ProcessDAppTransactionGroup simulates a transaction for a given DApp.
func (sm *ShardManager) ProcessDAppTransactionGroup(dappIDs []string) {
	for _, dappID := range dappIDs {
		sm.mu.RLock()
		dapp, ok := sm.Dapps[dappID]
		sm.mu.RUnlock()
		if !ok {
			fmt.Printf("Error: DApp %s not found.\n", dappID)
			return
		}
		dapp.ProcessTransaction()
	}
	sm.manageShards()
}

// RemoveDapp API method to remove a DApp from the network.
func (sm *ShardManager) RemoveDapp(dappID string) {
	sm.mu.RLock()
	shardID := sm.Hasher.GetShard(dappID)
	shard, ok := sm.Shards[shardID]
	sm.mu.RUnlock()

	if !ok {
		fmt.Printf("Error: Shard %s not found for DApp %s. Cannot remove.\n", shardID, dappID)
		return
	}
	shard.RemoveDApp(dappID)

	sm.mu.Lock()
	delete(sm.Dapps, dappID)
	sm.mu.Unlock()
	sm.manageShards()
}
