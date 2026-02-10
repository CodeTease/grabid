import React, { useState, useEffect, useRef } from 'react';
import { BrowserRouter, Routes, Route, Link } from 'react-router-dom';
import streamSaver from 'streamsaver';

// --- Types ---
interface ProbeResponse {
  size: number;
  type: string;
}

type AppState = 'IDLE' | 'PROBING' | 'STREAMING' | 'ERROR' | 'SUCCESS';

// --- Components ---

// Navbar Component
const Navbar = () => (
  <nav className="p-4 flex justify-between items-center border-b border-gray-800 bg-gray-900">
    <div className="text-xl font-bold text-white tracking-widest">GRABID</div>
    <div className="space-x-4">
      <Link to="/" className="text-gray-400 hover:text-white transition">Home</Link>
      <Link to="/docs" className="text-gray-400 hover:text-white transition">Docs</Link>
    </div>
  </nav>
);

// Progress Bar Component
const ProgressBar = ({ progress, speed, eta }: { progress: number; speed: string; eta: string }) => (
  <div className="w-full max-w-2xl mt-8 p-6 bg-gray-800 rounded-lg shadow-lg animate-fade-in">
    <div className="flex justify-between mb-2 text-sm text-gray-400">
      <span>Downloading...</span>
      <span>{progress.toFixed(1)}%</span>
    </div>
    <div className="w-full bg-gray-700 rounded-full h-2.5 overflow-hidden">
      <div 
        className="bg-blue-600 h-2.5 rounded-full transition-all duration-300 ease-out" 
        style={{ width: `${progress}%` }}
      ></div>
    </div>
    <div className="flex justify-between mt-2 text-xs text-gray-500">
      <span>Speed: {speed}</span>
      <span>ETA: {eta}</span>
    </div>
  </div>
);

// Error Component
const ErrorMessage = ({ message }: { message: string }) => (
  <div className="mt-6 p-4 bg-red-900/50 border border-red-700 text-red-200 rounded-lg max-w-2xl w-full text-center">
    Error: {message}
  </div>
);

// Home Page Component
const Home = () => {
  const [url, setUrl] = useState('');
  const [token, setToken] = useState(() => localStorage.getItem('grabid_token') || '');
  const [status, setStatus] = useState<AppState>('IDLE');
  const [error, setError] = useState('');
  const [progress, setProgress] = useState(0);
  const [speed, setSpeed] = useState('0 KB/s');
  const [eta, setEta] = useState('--');
  const [maxSizeLimit, setMaxSizeLimit] = useState<string>('');
  const abortControllerRef = useRef<AbortController | null>(null);

  useEffect(() => {
    localStorage.setItem('grabid_token', token);
  }, [token]);

  useEffect(() => {
    const fetchInfo = async () => {
      try {
        const res = await fetch('/api/v1/info', {
           headers: { 'X-Grab-Token': token },
        });
        if (res.ok) {
            const data = await res.json();
            if (data.max_size_limit) {
                setMaxSizeLimit(data.max_size_limit);
            }
        }
      } catch (e) {
        console.error("Failed to fetch info", e);
      }
    };
    fetchInfo();
  }, [token]);

  const handleDownload = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!url) return;

    setStatus('PROBING');
    setError('');
    setProgress(0);
    setSpeed('0 KB/s');
    setEta('--');

    try {
      // 1. Probe
      const probeRes = await fetch(`/api/v1/probe?url=${encodeURIComponent(url)}`, {
        method: 'GET', // Using GET as per implementation decision, though backend supports both
        headers: { 'X-Grab-Token': token },
      });

      if (!probeRes.ok) {
        throw new Error(`Probe failed: ${probeRes.status} ${probeRes.statusText}`);
      }

      const meta: ProbeResponse = await probeRes.json();
      const fileSize = meta.size;

      // 2. Stream
      setStatus('STREAMING');
      abortControllerRef.current = new AbortController();

      const response = await fetch(`/api/v1/stream?url=${encodeURIComponent(url)}`, {
        headers: { 'X-Grab-Token': token },
        signal: abortControllerRef.current.signal,
      });

      if (!response.ok) {
         if (response.status === 413) {
             throw new Error(`File vượt quá giới hạn cho phép (Max: ${maxSizeLimit || 'Unknown'})`);
         } else if (response.status === 429) {
             throw new Error("Bạn thao tác quá nhanh. Vui lòng thử lại sau vài giây.");
         } else if (response.status === 503) {
             throw new Error("Server đang bận (Full slots). Vui lòng đợi người khác tải xong.");
         }
         throw new Error(`Stream failed: ${response.status}`);
      }
      if (!response.body) throw new Error('ReadableStream not supported');

      const reader = response.body.getReader();
      
      // Determine filename
      const contentDisposition = response.headers.get('Content-Disposition');
      let filename = 'download';
      if (contentDisposition) {
        const match = contentDisposition.match(/filename="?([^"]+)"?/);
        if (match && match[1]) filename = match[1];
      } else {
        const urlParts = url.split('/');
        const lastPart = urlParts[urlParts.length - 1];
        if (lastPart) filename = lastPart;
      }

      // Initialize StreamSaver
      const fileStream = streamSaver.createWriteStream(filename, {
        size: fileSize > 0 ? fileSize : undefined // Pass size if known
      });
      const writer = fileStream.getWriter();

      // Tracking variables
      let receivedLength = 0;
      const startTime = Date.now();
      let lastUpdate = startTime;

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        await writer.write(value);
        receivedLength += value.length;

        // Update progress every 100ms
        const now = Date.now();
        if (now - lastUpdate > 100) {
          if (fileSize > 0) {
            setProgress((receivedLength / fileSize) * 100);
          }
          
          // Speed calculation
          const duration = (now - startTime) / 1000; // seconds
          const bps = receivedLength / duration;
          setSpeed(formatBytes(bps) + '/s');

          // ETA calculation
          if (fileSize > 0 && bps > 0) {
            const remainingBytes = fileSize - receivedLength;
            const remainingSeconds = remainingBytes / bps;
            setEta(formatTime(remainingSeconds));
          }

          lastUpdate = now;
        }
      }

      writer.close();
      setStatus('SUCCESS');
      setTimeout(() => setStatus('IDLE'), 3000); // Reset after 3s

    } catch (err: unknown) {
      if (err instanceof Error && err.name === 'AbortError') {
        setStatus('IDLE');
      } else {
        setError(err instanceof Error ? err.message : 'Unknown error occurred');
        setStatus('ERROR');
      }
    }
  };

  const cancelDownload = () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }
  };

  return (
    <div className="flex flex-col items-center justify-center min-h-[80vh] px-4">
      <div className="w-full max-w-2xl text-center mb-10">
        <h1 className="text-5xl font-extrabold mb-4 bg-clip-text text-transparent bg-gradient-to-r from-blue-400 to-purple-600">
          Grabid
        </h1>
        <p className="text-gray-400 text-lg">Stream-through any file, instantly.</p>
      </div>

      <form onSubmit={handleDownload} className="w-full max-w-2xl space-y-4">
        <div className="relative group">
          <input
            type="url"
            placeholder="Paste your link here..."
            className="w-full p-4 pl-6 text-lg bg-gray-800 border-2 border-gray-700 rounded-xl focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500/50 transition-all text-white placeholder-gray-500"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            disabled={status === 'STREAMING' || status === 'PROBING'}
            required
          />
          {status === 'IDLE' || status === 'ERROR' || status === 'SUCCESS' ? (
            <button
              type="submit"
              className="absolute right-2 top-2 bottom-2 px-6 bg-blue-600 hover:bg-blue-700 text-white font-medium rounded-lg transition-colors"
            >
              Grab
            </button>
          ) : (
            <button
              type="button"
              onClick={cancelDownload}
              className="absolute right-2 top-2 bottom-2 px-6 bg-red-600 hover:bg-red-700 text-white font-medium rounded-lg transition-colors"
            >
              Stop
            </button>
          )}
        </div>
        
        {maxSizeLimit && (
            <div className="text-gray-500 text-sm text-center">
                Max Size Limit: {maxSizeLimit}
            </div>
        )}

        {/* Token Input (Collapsible or just visible) */}
        <div className="flex justify-end">
           <input
            type="password"
            placeholder="Access Token (Optional)"
            className="p-2 bg-transparent border-b border-gray-700 text-gray-400 text-sm focus:border-blue-500 focus:outline-none w-48 text-right"
            value={token}
            onChange={(e) => setToken(e.target.value)}
          />
        </div>
      </form>

      {status === 'PROBING' && (
        <div className="mt-8 text-blue-400 animate-pulse">Probing source...</div>
      )}

      {status === 'STREAMING' && (
        <ProgressBar progress={progress} speed={speed} eta={eta} />
      )}

      {status === 'ERROR' && <ErrorMessage message={error} />}
      
      {status === 'SUCCESS' && (
         <div className="mt-8 text-green-400 font-bold">Download Complete!</div>
      )}
    </div>
  );
};

// Docs Page Component
const Docs = () => (
  <div className="max-w-4xl mx-auto p-8 text-gray-300">
    <h1 className="text-3xl font-bold text-white mb-6">API Documentation</h1>
    
    <section className="mb-8">
      <h2 className="text-xl font-semibold text-blue-400 mb-2">GET /api/v1/probe</h2>
      <p className="mb-2">Retrieves metadata about the source file.</p>
      <code className="block bg-gray-800 p-4 rounded-lg text-sm mb-4">
        GET /api/v1/probe?url=https://example.com/file.zip<br/>
        Header: X-Grab-Token: &lt;secret&gt;
      </code>
      <p>Response:</p>
      <pre className="bg-gray-800 p-4 rounded-lg text-sm text-green-400">
{`{
  "size": 1048576,
  "type": "application/zip"
}`}
      </pre>
    </section>

    <section className="mb-8">
      <h2 className="text-xl font-semibold text-blue-400 mb-2">GET /api/v1/stream</h2>
      <p className="mb-2">Streams the file content directly.</p>
      <code className="block bg-gray-800 p-4 rounded-lg text-sm">
        GET /api/v1/stream?url=https://example.com/file.zip<br/>
        Header: X-Grab-Token: &lt;secret&gt;
      </code>
    </section>
  </div>
);

// Helpers
const formatBytes = (bytes: number, decimals = 2) => {
  if (bytes === 0) return '0 Bytes';
  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
};

const formatTime = (seconds: number) => {
  if (!isFinite(seconds)) return '--';
  if (seconds < 60) return `${Math.floor(seconds)}s`;
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60);
  return `${m}m ${s}s`;
};

// Main App
function App() {
  return (
    <BrowserRouter>
      <div className="min-h-screen bg-gray-900 text-white font-sans selection:bg-blue-500/30">
        <Navbar />
        <main className="container mx-auto py-8">
          <Routes>
            <Route path="/" element={<Home />} />
            <Route path="/docs" element={<Docs />} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  );
}

export default App;
