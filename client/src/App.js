import { useEffect, useState } from "react";
import "./App.css";

const fetchData = (callback) => {
  console.log("FETCHING")
  return fetch("/api/torrents")
    .then((response) => response.json())
    .then((data) => callback(data))
    .catch((err) => console.log(`ERROR: ${err}`));
};

function App() {
  const [data, setData] = useState({});

  useEffect(() => {
    const id = setInterval(() => fetchData(setData), 3000);

    return () => {
      clearInterval(id);
    };
  }, []);

  return (
    <div className="App">
      <div>Connections: {data.connections ? data.connections : 0}</div>
      <ul
        style={{
          listStyle: "none",
          flex: 1,
          justifyContent: "flex-start",
          backgroundColor: "red",
          padding: 10,
        }}
      >
        {renderData(data)}
      </ul>
    </div>
  );
}

const formatFileSize = (size = 0) => {
  if (size < 1024) return [size, "B"];
  if (size < Math.pow(1024, 2)) {
    return [(size / 1024).toFixed(2), "KiB"];
  }
  if (size < Math.pow(1024, 3)) {
    return [(size / Math.pow(1024, 2)).toFixed(2), "MiB"];
  }

  if (size < Math.pow(1024, 4)) {
    return [(size / Math.pow(1024, 3)).toFixed(2), "GiB"];
  }
};

const renderFile = (file = {}) => {
  const [downloaded, downloadedUnit] = formatFileSize(file.Downloaded);
  const [totalSize, totalSizeUnit] = formatFileSize(file.Size);
  return (
    <li style={{ marginBottom: 50 }}>
      <div>
        {file.Name} ({file.Index})
      </div>
      <div>Size: {`${totalSize} ${totalSizeUnit}`}</div>
      <div>Downloaded: {`${downloaded} ${downloadedUnit}`}</div>
      <div>Ignored: {`${file.Ignored}`}</div>
    </li>
  );
};

const renderData = (data = {}) => {
  return (
    <>
      {Object.entries(data?.clients ?? {}).map(([hash, client]) => {
        const [downloaded, downloadedUnit] = formatFileSize(client.downloaded);
        const [downloadRate, downloadRateUnit] = formatFileSize(
          client.downloadRate
        );
        const [length, lengthUnit] = formatFileSize(client.total);
        const swarm = data?.swarms[hash];

        const timeLeft = new Date((client.total / client.downloadRate) * 1000)
          .toISOString()
          .substr(11, 8);

        return (
          <li
            style={{
              backgroundColor: "grey",
              flex: 1,
              justifyContent: "flex-start",
              textAlign: "start",
            }}
          >
            <div>{client.name}</div>
            <div>Time left: {timeLeft}</div>
            <div>
              Peers: {swarm.Peers.length} ({swarm.Blocking} Choked by,{" "}
              {swarm.Interesting} Interesting, {swarm.Interested} Interested,{" "}
              {swarm.Choked} Choked)
            </div>
            <div>
              Downloaded: {`${downloaded} ${downloadedUnit}`} /{" "}
              {`${length} ${lengthUnit}`}
            </div>
            <div>
              Download Rate: {`${downloadRate} ${downloadRateUnit}`} / s
            </div>
            <ul style={{ listStyle: "none", marginTop: 50 }}>
              {client?.files
                ?.filter((e) => !e.Ignored)
                //.sort((e1, e2) => e1.Name.localeCompare(e2.Name))
                .map(renderFile) ?? null}
            </ul>
          </li>
        );
      })}
    </>
  );
};

export default App;
