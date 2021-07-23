import logo from "./logo.svg";
import "./App.css";
import { useEffect, useState } from "react";

function msToTime(duration) {
    var milliseconds = parseInt((duration%1000)/100)
    , seconds = parseInt((duration/1000)%60)
    , minutes = parseInt((duration/(1000*60))%60)
    , hours = parseInt((duration/(1000*60*60))%24);

   hours = (hours < 10) ? "0" + hours : hours;
   minutes = (minutes < 10) ? "0" + minutes : minutes;
   seconds = (seconds < 10) ? "0" + seconds : seconds;

   return hours + ":" + minutes + ":" + seconds + "." + milliseconds;
}

const renderSwarms = (torrents = [], trackers = {}) => (swarm) => {
  console.log({torrents, swarm, trackers})
  const torrentIndex = torrents.reduce((acc, item) => {
    acc[item.infoHash] = item
    return acc
  }, {})

  const aggTracker = trackers[swarm.torrent].reduce((acc, item) => {
    acc.leechers += item.leechers
    acc.seeders += item.seeders
    return acc
  }, { leechers: 0, seeders: 0 })


  //console.log("STATE", {
    //choking: swarm.choking,
    //blocking: swarm.blocking,
    //interested: swarm.interested,
    //interesting: swarm.interesting,
    //peers: swarm.npeers,
    //idle: swarm.idle
  //})
  //console.log("stats", swarm.stats)
  
  const stats = swarm.stats
  const torrent = torrentIndex[swarm.torrent]

  return <tr key={swarm.torrent}>
        <td>{torrent?.name ?? "LOL"}</td>
        <td>{0}</td>
        <td>{stats?.downloadRate ?? 0}</td>
        <td>{stats?.uploaded ?? 0}</td>
        <td>{swarm.npeers}</td>
        <td>{Math.floor(aggTracker.leechers / trackers[swarm.torrent].length)}</td>
        <td>{Math.floor(aggTracker.seeders / trackers[swarm.torrent].length)}</td>
        <td>{`${(swarm.have / swarm.npieces * 100).toFixed(2)} %`}</td>
      </tr>
}

function renderSession(data = {}) {
  //if (!data.trackers || !data.torrents || !data.swarms ){
    //console.log("STILL LOADING")
    //return null
  //}

  return <div>
    <header> Session: {msToTime(data.sessionLength / 1000000)} </header>
        <table style={{width: "100%"}}>
        <tr>
          <th>Torrent</th>
          <th>Download</th>
          <th>Download rate (B / s)</th>
          <th>Uploaded</th>
          <th>Peers</th>
          <th>Leechers</th>
          <th>Seeders</th>
          <th>Progress</th>
        </tr>
      {
        data.swarms && data.swarms.map(renderSwarms(data.torrents, data.trackers))
      }
        </table>
  </div>
}

function App() {
  const [data, setData] = useState({})
  useEffect(() => {
    const id = setInterval(() => {
      fetch("http://localhost:8080/").then((data) => {
        return data.json()
      })
        .then(setData)
        .catch(console.error)
    }, 1000)

    return () => clearInterval(id)
  }, []);

  return (
    <div className="App">
      <div style={{ color: "red", backgroundColor: "black" }}>
          {renderSession(data)}
      </div>
    </div>
  );
}

export default App;
