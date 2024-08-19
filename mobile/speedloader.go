package main

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/breez/breez/refcount"
	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/btcsuite/btcwallet/walletdb/bdb"
	"github.com/gabstv/go-bsdiff/pkg/bspatch"
	"github.com/lightningnetwork/lnd/channeldb"
	"github.com/lightningnetwork/lnd/channeldb/models"
	"go.etcd.io/bbolt"
)

const (
	directoryPattern = "data/graph/{{network}}/"
)

var (
	ErrMissingPolicyError = errors.New("missing channel policy")
	serviceRefCounter     refcount.ReferenceCountable
	chanDB                *channeldb.DB
	bucketsToCopy         = map[string]struct{}{
		"graph-edge": {},
		"graph-node": {},
	}
	globalCtx    context.Context
	cancelGlobal context.CancelFunc
)

type Logger struct {
	file *os.File
	mu   sync.Mutex // Ensures thread safety
}

func init() {
	// Initialize the global context
	globalCtx, cancelGlobal = context.WithCancel(context.Background())
}

func NewLogger(path string) (*Logger, error) {
	logFile, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0777)
	if err != nil {
		return nil, err
	}
	return &Logger{file: logFile}, nil
}

func (l *Logger) Println(message string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err := l.file.WriteString(message + "\n")
	if err != nil {
		return err
	}
	return nil
}

func (l *Logger) Printf(format string, v ...interface{}) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err := l.file.WriteString(fmt.Sprintf(format, v...) + "\n")
	if err != nil {
		return err
	}
	return nil
}

func (l *Logger) Close() error {
	return l.file.Close()
}

func createService(workingDir string, log *Logger) (*channeldb.DB, error) {
	var err error
	graphDir := path.Join(workingDir, strings.Replace(directoryPattern, "{{network}}", "mainnet", -1))
	log.Println("creating shared channeldb service.")
	chanDB, err := channeldb.Open(graphDir,
		channeldb.OptionSetSyncFreelist(true))
	if err != nil {
		log.Printf("unable to open channeldb: %v", err)
		return nil, err
	}

	log.Println("channeldb was opened successfuly")
	return chanDB, err
}

func release() error {
	return chanDB.Close()
}

func newService(workingDir string, log *Logger) (db *channeldb.DB, rel refcount.ReleaseFunc, err error) {
	chanDB, err = createService(workingDir, log)
	if err != nil {
		return nil, nil, err
	}
	return chanDB, release, err
}

func walkBucket(b *bbolt.Bucket, keypath [][]byte, k, v []byte, seq uint64, fn walkFunc, skip skipFunc, log *Logger) error {
	select {
	case <-globalCtx.Done():
		// Global context cancelled, return early
		log.Println("Cancelling walkBucket")
		return globalCtx.Err()
	default:
		if skip != nil && skip(keypath, k, v) {
			return nil
		}
		// Execute callback.
		if err := fn(keypath, k, v, seq); err != nil {
			return err
		}
		// If this is not a bucket then stop.
		if v != nil {
			return nil
		}
		// Iterate over each child key/value.
		keypath = append(keypath, k)
		return b.ForEach(func(k, v []byte) error {
			if v == nil {
				bkt := b.Bucket(k)
				return walkBucket(bkt, keypath, k, nil, bkt.Sequence(), fn, skip, log)
			}
			return walkBucket(b, keypath, k, v, b.Sequence(), fn, skip, log)
		})
	}
}

func walk(db *bbolt.DB, walkFn walkFunc, skipFn skipFunc, log *Logger) error {
	select {
	case <-globalCtx.Done():
		// Global context cancelled, return early
		log.Println("Cancelling walk")
		return globalCtx.Err()
	default:
		return db.View(func(tx *bbolt.Tx) error {
			return tx.ForEach(func(name []byte, b *bbolt.Bucket) error {
				return walkBucket(b, nil, name, nil, b.Sequence(), walkFn, skipFn, log)
			})
		})
	}
}

// Merge copies from source to dest and ignoring items using the skip function.
// It is different from Compact in that it tries to create a bucket only if not exists.
func merge(tx *bbolt.Tx, src *bbolt.DB, skip skipFunc, log *Logger) error {
	select {
	case <-globalCtx.Done():
		// Global context cancelled, return early
		log.Println("Cancelling merge")
		return globalCtx.Err()
	default:
		if err := walk(src, func(keys [][]byte, k, v []byte, seq uint64) error {
			// Create bucket on the root transaction if this is the first level.
			nk := len(keys)
			if nk == 0 {
				bkt, err := tx.CreateBucketIfNotExists(k)
				if err != nil {
					return err
				}
				if err := bkt.SetSequence(seq); err != nil {
					return err
				}
				return nil
			}
			// Create buckets on subsequent levels, if necessary.
			b := tx.Bucket(keys[0])
			if nk > 1 {
				for _, k := range keys[1:] {
					b = b.Bucket(k)
				}
			}
			// Fill the entire page for best compaction.
			b.FillPercent = 1.0
			// If there is no value then this is a bucket call.
			if v == nil {
				bkt, err := b.CreateBucketIfNotExists(k)
				if err != nil {
					return err
				}
				if err := bkt.SetSequence(seq); err != nil {
					return err
				}
				return nil
			}
			// Otherwise treat it as a key/value pair.
			return b.Put(k, v)
		}, skip, log); err != nil {
			return err
		}
		return nil
	}
}

type walkFunc func(keys [][]byte, k, v []byte, seq uint64) error

type skipFunc func(keys [][]byte, k, v []byte) bool

func ourNode(chanDB *channeldb.DB) (*channeldb.LightningNode, error) {
	graph := chanDB.ChannelGraph()
	node, err := graph.SourceNode()
	if err == channeldb.ErrSourceNodeNotSet || err == channeldb.ErrGraphNotFound {
		return nil, nil
	}
	return node, err
}

func ourData(chanDB *channeldb.DB, ourNode *channeldb.LightningNode, log *Logger) (
	[]*channeldb.LightningNode, []*models.ChannelEdgeInfo, []*models.ChannelEdgePolicy, error) {
	nodeMap := make(map[string]*channeldb.LightningNode)
	var edges []*models.ChannelEdgeInfo
	var policies []*models.ChannelEdgePolicy
	var nodes []*channeldb.LightningNode

	select {
	case <-globalCtx.Done():
		// Global context cancelled, return early
		log.Println("Cancelling ourData")
		return nodes, edges, policies, globalCtx.Err()
	default:
		graph := chanDB.ChannelGraph()
		err := graph.ForEachNodeChannel(ourNode.PubKeyBytes, func(tx walletdb.ReadTx,
			channelEdgeInfo *models.ChannelEdgeInfo,
			toPolicy *models.ChannelEdgePolicy,
			fromPolicy *models.ChannelEdgePolicy) error {

			if toPolicy == nil || fromPolicy == nil {
				return nil
			}
			nodeMap[hex.EncodeToString(toPolicy.ToNode[:])] = &channeldb.LightningNode{
				PubKeyBytes: toPolicy.ToNode,
			}
			edges = append(edges, channelEdgeInfo)
			if toPolicy != nil {
				policies = append(policies, toPolicy)
			}
			if fromPolicy != nil {
				policies = append(policies, fromPolicy)
			}
			return nil
		})

		if err != nil {
			return nil, nil, nil, err
		}
		for _, node := range nodeMap {
			nodes = append(nodes, node)
		}
		return nodes, edges, policies, nil
	}
}

func putOurData(chanDB *channeldb.DB, node *channeldb.LightningNode, nodes []*channeldb.LightningNode,
	edges []*models.ChannelEdgeInfo, policies []*models.ChannelEdgePolicy, log *Logger) error {

	select {
	case <-globalCtx.Done():
		// Global context cancelled, return early
		log.Println("Cancelling putOurData")
		return globalCtx.Err()
	default:
		graph := chanDB.ChannelGraph()
		err := graph.SetSourceNode(node)
		if err != nil {
			return fmt.Errorf("graph.SetSourceNode(%x): %w", node.PubKeyBytes, err)
		}
		for _, n := range nodes {
			err = graph.AddLightningNode(n)
			if err != nil {
				return fmt.Errorf("graph.AddLightningNode(%x): %w", n.PubKeyBytes, err)
			}
		}
		for _, edge := range edges {
			err = graph.AddChannelEdge(edge)
			if err != nil && err != channeldb.ErrEdgeAlreadyExist {
				return fmt.Errorf("graph.AddChannelEdge(%x): %w", edge.ChannelID, err)
			}
		}
		for _, policy := range policies {
			err = graph.UpdateEdgePolicy(policy)
			if err != nil {
				return fmt.Errorf("graph.UpdateEdgePolicy(): %w", err)
			}
		}
		return nil
	}
}

func hasSourceNode(tx *bbolt.Tx) bool {
	nodes := tx.Bucket([]byte("graph-node"))
	if nodes == nil {
		return false
	}
	selfPub := nodes.Get([]byte("source"))
	return selfPub != nil
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func initDirs(cacheDir string) {
	os.MkdirAll(cacheDir+"/dgraph", 0777)
	os.MkdirAll(cacheDir+"/usage", 0777)
	os.MkdirAll(cacheDir+"/log", 0777)
}

func downloadPatch(cacheDir string, log *Logger, dgraphHash string, patchURL string) error {
	patchPath := cacheDir + "/graph-patch"
	log.Printf("Downloading patch for %s", dgraphHash)

	select {
	case <-globalCtx.Done():
		// Global context cancelled, return early
		log.Println("Cancelling downloadPatch")
		return globalCtx.Err()
	default:
		out, err := os.Create(patchPath)
		if err != nil {
			log.Println(err.Error())
			return err
		}
		client := new(http.Client)
		req, err := http.NewRequest("GET", patchURL+dgraphHash+"/graph-patch", nil)
		if err != nil {
			log.Println(err.Error())
			return err
		}
		req.Header.Add("Accept-Encoding", "br, gzip")
		req = req.WithContext(globalCtx)
		resp, err := client.Do(req)
		if err != nil {
			log.Println(err.Error())
			return err
		}
		var reader io.Reader
		switch resp.Header.Get("Content-Encoding") {
		case "gzip":
			reader, err = gzip.NewReader(resp.Body)
			if err != nil {
				log.Println(err.Error())
				return err
			}
		case "br":
			reader = brotli.NewReader(resp.Body)
		default:
			reader = resp.Body
		}
		if resp.StatusCode == 200 {
			byteCt, err := io.Copy(out, reader)
			log.Printf("%d bytes written for patch", byteCt)
			fi, err := os.Stat(patchPath)
			if err != nil {
				return err
			}
			log.Printf("%d bytes on disk for patch", fi.Size())
			if err != nil {
				log.Println(err.Error())
				return err
			}
			out.Close()
			resp.Body.Close()
			return nil
		} else {
			return errors.New("failed to download patch")
		}
	}
}

func downloadGraph(cacheDir string, dgraphPath string, log *Logger, breezURL string) error {
	log.Println("Creating new dgraph")

	select {
	case <-globalCtx.Done():
		// Global context cancelled, return early
		log.Println("Cancelling downloadGraph")
		return globalCtx.Err()
	default:
		out, err := os.Create(dgraphPath)
		if err != nil {
			log.Println(err.Error())
			return err
		}
		client := new(http.Client)
		req, err := http.NewRequest("GET", breezURL, nil)
		if err != nil {
			log.Println(err.Error())
			return err
		}
		req.Header.Add("Accept-Encoding", "br, gzip")
		req = req.WithContext(globalCtx)
		resp, err := client.Do(req)
		if err != nil {
			log.Println(err.Error())
			return err
		}
		var reader io.Reader
		switch resp.Header.Get("Content-Encoding") {
		case "gzip":
			reader, err = gzip.NewReader(resp.Body)
			if err != nil {
				log.Println(err.Error())
				return err
			}
		case "br":
			reader = brotli.NewReader(resp.Body)
		default:
			reader = resp.Body
		}
		byteCt, err := io.Copy(out, reader)
		log.Printf("%d bytes written", byteCt)
		fi, err := os.Stat(dgraphPath)
		if err != nil {
			return err
		}
		log.Printf("%d bytes on disk", fi.Size())
		if err != nil {
			log.Println(err.Error())
			return err
		}
		out.Close()
		resp.Body.Close()
		return nil
	}
}

func GossipSync(serviceUrl string, cacheDir string, dataDir string, networkType string, callback Callback) {
	var (
		firstRun      bool
		useDGraph     bool
		dgraphPath    = cacheDir + "/dgraph/channel.db"
		logPath       = cacheDir + "/log/speedloader.log"
		usagePath     = cacheDir + "/usage/channel.db"
		breezURL      = serviceUrl + "/mainnet/graph/graph-001d.db"
		patchURL      = serviceUrl + "/mainnet/graph/"
		checksumURL   = serviceUrl + "/mainnet/graph/MD5SUMS"
		checksumValue string
	)

	initDirs(cacheDir)
	log, err := NewLogger(logPath)
	if err != nil {
		callback.OnError(err)
		return
	}

	select {
	case <-globalCtx.Done():
		// Global context cancelled, return early
		log.Println("Cancelling GossipSync")
		callback.OnError(globalCtx.Err())
		return
	default:
		// check lastRun time, return early if we ran too recently
		lastRunPath := cacheDir + "/lastrun"
		if !fileExists(lastRunPath) {
			os.Create(lastRunPath)
			firstRun = true
		}
		lastRun, err := os.Stat(lastRunPath)
		if err == nil {
			modifiedTime := lastRun.ModTime()
			now := time.Now()
			diff := now.Sub(modifiedTime)
			if !firstRun && diff.Hours() <= 24 {
				// this is not the first run and
				// we have run speedloader within the last 24h, abort
				callback.OnResponse([]byte("skip_time_constraint"))
				return
			}
		}
		// checksum fetching
		client := new(http.Client)
		req, err := http.NewRequest("GET", checksumURL, nil)
		if err != nil {
			callback.OnError(err)
			return
		}
		req = req.WithContext(globalCtx)
		resp, err := client.Do(req)
		if err != nil {
			callback.OnError(err)
			return
		}
		defer resp.Body.Close()
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					break
				} else {
					callback.OnError(err)
					return
				}
			}
			fields := strings.Fields(line)
			if len(fields) != 2 {
				callback.OnError(errors.New("unexpected_checksum_line_fmt"))
				return
			}
			filename := fields[1]
			hash := fields[0]
			if filename == "graph-001d.db" {
				checksumValue = hash
			}
		}
		// In light of the new incremental mode, I'm disabling this
		// if networkType != "wifi" && networkType != "ethernet" {
		// 	useDGraph = true
		// }
		if checksumValue != "" {
			// we have a valid checksum
			// do we have a file?
			if fileExists(dgraphPath) {
				log.Println("Found existing dgraph")
				// first, calculate the md5sum of the file we have
				var dgraphBytes []byte
				md5h := md5.New()
				fh, _ := os.Open(dgraphPath)
				_, err = io.Copy(md5h, fh)
				if err != nil {
					callback.OnError(err)
					return
				}
				defer fh.Close()
				sum := md5h.Sum(nil)
				calculatedChecksum := hex.EncodeToString(sum)
				// Seek to beginning
				_, err = fh.Seek(0, 0)
				if err != nil {
					callback.OnError(err)
					return
				}
				dgraphBytes, err := ioutil.ReadAll(fh)
				if err != nil {
					callback.OnError(err)
					return
				}
				defer fh.Close()
				// bsdiff download attempt
				err = downloadPatch(cacheDir, log, calculatedChecksum, patchURL)
				if err == nil {
					// patched graph checksum
					patchPath := cacheDir + "/graph-patch"
					patch, err := os.Open(patchPath)
					patchBytes, err := ioutil.ReadAll(patch)
					if err != nil {
						callback.OnError(err)
						return
					}
					patch.Close()
					// apply patch
					log.Println("Patching existing dgraph")
					newGraph, err := bspatch.Bytes(dgraphBytes, patchBytes)
					if err != nil {
						callback.OnError(err)
						return
					}
					err = ioutil.WriteFile(dgraphPath, newGraph, 0777)
					if err != nil {
						callback.OnError(err)
						return
					}
					log.Println("Patched existing dgraph")
					// Recalculate hash
					md5h := md5.New()
					ph, _ := os.Open(dgraphPath)
					_, err = io.Copy(md5h, ph)
					if err != nil {
						callback.OnError(err)
						return
					}
					defer ph.Close()
					sum := md5h.Sum(nil)
					calculatedChecksum = hex.EncodeToString(sum)
				} else {
					log.Printf("Patch failed: %s", err)
				}

				if checksumValue != calculatedChecksum {
					// failed checksum check (existing file)
					// unconditionally try to delete dgraph file and lastRun
					log.Printf("Checksum for existing mismatch. Expected %s got %s, retrying download", checksumValue, calculatedChecksum)
					os.Remove(dgraphPath)
					os.Remove(lastRunPath)
					if !useDGraph {
						// checksum fetching
						client := new(http.Client)
						req, err := http.NewRequest("GET", checksumURL, nil)
						if err != nil {
							callback.OnError(err)
							return
						}
						req = req.WithContext(globalCtx)
						resp, err := client.Do(req)
						if err != nil {
							callback.OnError(err)
							return
						}
						defer resp.Body.Close()
						reader := bufio.NewReader(resp.Body)
						for {
							line, err := reader.ReadString('\n')
							if err != nil {
								if err == io.EOF {
									break
								} else {
									callback.OnError(err)
									return
								}
							}
							fields := strings.Fields(line)
							if len(fields) != 2 {
								callback.OnError(errors.New("unexpected_checksum_line_fmt"))
								return
							}
							filename := fields[1]
							hash := fields[0]
							if filename == "graph-001d.db" {
								checksumValue = hash
							}
						}
						// download retry
						err = downloadGraph(cacheDir, dgraphPath, log, breezURL)
						if err != nil {
							callback.OnError(err)
							return
						}
						// recalculate the md5sum of the retry downloaded file we have
						fh, err := os.Open(dgraphPath)
						if err != nil {
							callback.OnError(err)
							return
						}
						defer fh.Close()
						md5h := md5.New()
						_, err = io.Copy(md5h, fh)
						if err != nil {
							callback.OnError(err)
							return
						}
						sum := md5h.Sum(nil)
						calculatedChecksum = hex.EncodeToString(sum)
						if checksumValue != calculatedChecksum {
							// failed checksum check again
							// unconditionally remove dgraph file and lastRun and give up
							log.Printf("Checksum mismatch for redownload, expected %s got %s", checksumValue, calculatedChecksum)
							os.Remove(dgraphPath)
							os.Remove(lastRunPath)
							callback.OnResponse([]byte("skip_checksum_failed"))
							return
						} else {
							log.Printf("Checksum validated on redownload: %s", calculatedChecksum)
							useDGraph = true
						}
					}
				} else {
					log.Printf("Checksum OK %s", calculatedChecksum)
					// checksum matches
					// now check modtime
					info, err := os.Stat(dgraphPath)
					// check the modified time on the existing downloaded channel.db, see if it is <= 48h old
					if err == nil {
						modifiedTime := info.ModTime()
						now := time.Now()
						diff := now.Sub(modifiedTime)
						if diff.Hours() <= 48 {
							// abort downloading the graph, we have a fresh-enough downloaded graph
							useDGraph = true
						}
					}
				}
			}
		}
		// if the dgraph is not usable, download the graph
		if !useDGraph {
			// download the breez gossip database
			err = downloadGraph(cacheDir, dgraphPath, log, breezURL)
			if err != nil {
				callback.OnError(err)
				return
			}
			fh, err := os.Open(dgraphPath)
			if err != nil {
				callback.OnError(err)
				return
			}
			defer fh.Close()
			md5h := md5.New()
			_, err = io.Copy(md5h, fh)
			if err != nil {
				callback.OnError(err)
				return
			}
			sum := md5h.Sum(nil)
			calculatedChecksum := hex.EncodeToString(sum)
			if checksumValue != calculatedChecksum {
				// failed checksum check (just downloaded file)
				// unconditionally remove dgraph file and lastRun
				if err != nil {
					callback.OnError(err)
					return
				}
				log.Printf("Checksum mismatch, expected %s got %s", checksumValue, calculatedChecksum)
				os.Remove(dgraphPath)
				os.Remove(lastRunPath)
				callback.OnResponse([]byte("skip_checksum_failed"))
				return
			} else {
				log.Printf("Checksum OK %s", calculatedChecksum)
			}
		}
		// open channel.db as dest
		service, release, err := serviceRefCounter.Get(
			func() (interface{}, refcount.ReleaseFunc, error) {
				return newService(dataDir, log)
			},
		)
		if err != nil {
			callback.OnError(err)
			return
		}
		defer release()
		destDB := service.(*channeldb.DB)
		// temporarily copy dgraph to usage dir
		err = copyFile(dgraphPath, usagePath, log)
		// open dgraph.db as source
		dchanDB, err := channeldb.Open(cacheDir + "/usage")
		defer os.Remove(usagePath)
		defer dchanDB.Close()
		if err != nil {
			callback.OnError(err)
			return
		}
		defer dchanDB.Close()
		sourceDB, err := bdb.UnderlineDB(dchanDB.Backend)
		if err != nil {
			callback.OnError(err)
			return
		}
		// utility function to convert bolts key to a string path.
		extractPathElements := func(bytesPath [][]byte, key []byte) []string {
			var path []string
			for _, b := range bytesPath {
				path = append(path, string(b))
			}
			return append(path, string(key))
		}
		ourNode, err := ourNode(destDB)
		if err != nil {
			callback.OnError(err)
			return
		}
		kvdbTx, err := destDB.BeginReadWriteTx()
		if err != nil {
			callback.OnError(err)
			return
		}
		tx, err := bdb.UnderlineTX(kvdbTx)
		if err != nil {
			callback.OnError(err)
			return
		}
		defer tx.Rollback()
		if ourNode == nil && hasSourceNode(tx) {
			callback.OnError(err)
			return
			//errors.New("source node was set before sync transaction, rolling back").Error()
		}
		if ourNode != nil {
			channelNodes, channels, policies, err := ourData(destDB, ourNode, log)
			if err != nil {
				callback.OnError(err)
				return
			}

			// add our data to the source db.
			if err := putOurData(dchanDB, ourNode, channelNodes, channels, policies, log); err != nil {
				callback.OnError(err)
				return
			}
		}
		// clear graph data from the destination db
		for b := range bucketsToCopy {
			if err := tx.DeleteBucket([]byte(b)); err != nil && err != bbolt.ErrBucketNotFound {
				callback.OnError(err)
				return
			}
		}
		err = merge(tx, sourceDB,
			func(keyPath [][]byte, k []byte, v []byte) bool {
				pathElements := extractPathElements(keyPath, k)
				_, shouldCopy := bucketsToCopy[pathElements[0]]
				return !shouldCopy
			}, log)
		if err != nil {
			callback.OnError(err)
			return
		}
		// update the lastrun modified time
		now := time.Now()
		if !fileExists(lastRunPath) {
			_, err = os.Create(lastRunPath)
			if err != nil {
				callback.OnError(err)
				return
			}
		}
		err = os.Chtimes(lastRunPath, now, now)
		if err != nil {
			callback.OnError(err)
			return
		}
		callback.OnResponse([]byte(fmt.Sprintf("dl=%t,done_commit_err=%v", !useDGraph, tx.Commit())))
	}
}

func CancelGossipSync() {
	cancelGlobal()
}

func copyFile(srcPath, destPath string, log *Logger) error {
	select {
	case <-globalCtx.Done():
		// Global context cancelled, return early
		log.Println("Cancelling copyFile")
		return globalCtx.Err()
	default:
		srcFile, err := os.Open(srcPath)
		if err != nil {
			return err
		}
		defer srcFile.Close()
		destFile, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer destFile.Close()
		_, err = io.Copy(destFile, srcFile)
		if err != nil {
			return err
		}
		err = destFile.Sync()
		if err != nil {
			return err
		}
		return nil
	}
}
