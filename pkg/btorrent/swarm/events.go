package swarm

// func (s *Swarm) handleMulticastMessage(req MulticastMessage) (bool, error) {
// 	go func() {
// 		var res []*peer.Peer
// 		count := 0
//
// 		var cpy []*peer.Peer
// 		for _, p := range s.peers {
// 			cpy = append(cpy, p)
// 		}
//
// 		if req.OrderBy != nil {
// 			sort.SliceStable(cpy, func(i, j int) bool {
// 				return req.OrderBy(cpy[i], cpy[j]) < 0
// 			})
//
// 		} else {
// 			rand.Seed(time.Now().UnixNano())
// 			rand.Shuffle(len(cpy), func(i, j int) {
// 				cpy[i], cpy[j] = cpy[j], cpy[i]
// 			})
// 		}
//
// 		for _, peer := range cpy {
// 			n := rand.Int31n(100)
// 			if n < 25 {
// 				continue
// 			}
// 			if req.Limit > 0 && count == req.Limit {
// 				break
// 			}
//
// 			if req.Filter != nil && !req.Filter(peer) {
// 				continue
// 			}
//
// 			res = append(res, peer)
// 			count++
// 		}
//
// 		req.Handler(res)
// 	}()
//
// 	return false, nil
// }
