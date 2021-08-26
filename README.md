# bitsy

**WIP**: bitsy is a simple bittorrent client that I wrote as a learning experience and to better understand the BitTorrent P2P protocol. It implements only the core parts of the spec (BEP-3) needed for basic functionality (download/upload). It doesn't implement any clever piece selection strategy (the next piece to download is picked at random) such as rarest-first, or the tit-for-tat strategy when selecting which peers to unchoke.

I've only tested this on my own local setup, so it's unlikely to work on other systems as is, but in theory it can be used as follows:
```
go install github.com/namvu9/bitsy/bitsy

bitsy download <torrent file|magnet url>
bitsy download --files 0,1 <torrent|magnet> # Download a subset of files in a torrent
```

![bitsy](https://user-images.githubusercontent.com/66156529/129764420-714862cc-5e34-497e-9b60-158da081f122.png)

I've used bitsy to download small torrents (a few hundred MBs), but it struggles with larger files, often stalling. It also (sometimes) struggles during the initial stage when first joining a swarm during which the client does not have any pieces to reciprocate to other peers implementing the tit-for-tat algorithm. The client therefore struggles to stay unchoked by other peers.

* Only supports UDP trackers. HTTP trackers are not supported, but most trackers found in the wild are UDP trackers anyway.
* Supports magnet links (BEP-9)
* Only supports a single download at a time, but each download can be resumed (by invoking the client with the same torrent/magnet link).

Bitsy also comes with a few utilities. For example, the following command converts a magnet url to a torrent file:

```
bitsy getTorrent <url> > x.torrent
```
