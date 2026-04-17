import { useEffect, useState, useCallback, useRef } from 'react'
import { useRunStore } from '../../stores/run-store'
import { useAIStore } from '../../stores/ai-store'
import type { Run, TimeSeriesPoint, RunEvent } from '../../stores/run-store'
import { MetricCard } from './MetricCard'
import { LiveChart } from './LiveChart'
import { LiveControlPanel } from './LiveControlPanel'
import { RunCompletionBanner } from './RunCompletionBanner'
import { AnalyseButton } from '../ai/AnalyseButton'
import { AIInsightsPanel } from '../ai/AIInsightsPanel'

interface RunViewProps {
  runId: string
  onBack: () => void
}

function statusBadge(status: string) {
  const colors: Record<string, string> = {
    pending: 'bg-yellow-500/20 text-yellow-400',
    running: 'bg-teal-500/20 text-teal-400',
    completed: 'bg-green-500/20 text-green-400',
    failed: 'bg-red-500/20 text-red-400',
    cancelled: 'bg-slate-500/20 text-slate-400',
  }
  return colors[status] || 'bg-slate-500/20 text-slate-400'
}

function formatDuration(startedAt: string | null, endedAt: string | null): string {
  if (!startedAt) return '--'
  const start = new Date(startedAt).getTime()
  const end = endedAt ? new Date(endedAt).getTime() : Date.now()
  const seconds = Math.floor((end - start) / 1000)
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  return `${m}m ${s}s`
}

function downloadFile(filename: string, content: string, mimeType: string) {
  const blob = new Blob([content], { type: mimeType })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

function metricsToCSV(metrics: {
  rps: TimeSeriesPoint[]
  p95: TimeSeriesPoint[]
  errorRate: TimeSeriesPoint[]
  activeVUs: TimeSeriesPoint[]
}): string {
  const maxLen = Math.max(
    metrics.rps.length,
    metrics.p95.length,
    metrics.errorRate.length,
    metrics.activeVUs.length,
  )
  const rows: string[] = ['time,rps,p95,errorRate,activeVUs']
  for (let i = 0; i < maxLen; i++) {
    const time = metrics.rps[i]?.time ?? metrics.p95[i]?.time ?? metrics.errorRate[i]?.time ?? metrics.activeVUs[i]?.time ?? ''
    const rps = metrics.rps[i]?.value ?? ''
    const p95 = metrics.p95[i]?.value ?? ''
    const errorRate = metrics.errorRate[i]?.value ?? ''
    const activeVUs = metrics.activeVUs[i]?.value ?? ''
    rows.push(`${time},${rps},${p95},${errorRate},${activeVUs}`)
  }
  return rows.join('\n')
}

function generateStandaloneHTML(
  run: Run,
  metrics: {
    rps: TimeSeriesPoint[]
    p95: TimeSeriesPoint[]
    errorRate: TimeSeriesPoint[]
    activeVUs: TimeSeriesPoint[]
  },
  events: RunEvent[],
): string {
  const data = JSON.stringify({ run, metrics, events })
  return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Run Report - ${run.id.slice(0, 8)}</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:Inter,-apple-system,system-ui,sans-serif;background:#020617;color:#e2e8f0;padding:24px}
h1{font-size:20px;font-weight:600;margin-bottom:8px}
.meta{font-size:12px;color:#94a3b8;margin-bottom:24px}
.badge{display:inline-block;padding:2px 10px;border-radius:9999px;font-size:12px;font-weight:500}
.badge-completed{background:rgba(16,185,129,0.2);color:#34d399}
.badge-failed{background:rgba(239,68,68,0.2);color:#f87171}
.badge-running{background:rgba(234,179,8,0.2);color:#facc15}
.badge-cancelled{background:rgba(100,116,139,0.2);color:#94a3b8}
.badge-pending{background:rgba(234,179,8,0.2);color:#facc15}
.cards{display:grid;grid-template-columns:repeat(3,1fr);gap:12px;margin-bottom:24px}
.card{background:#0f172a;border:1px solid #1e293b;border-radius:8px;padding:12px 16px}
.card-label{font-size:11px;color:#64748b}
.card-value{font-size:24px;font-weight:600;margin-top:4px}
.chart-section{margin-bottom:24px}
.chart-title{font-size:13px;font-weight:500;color:#cbd5e1;margin-bottom:8px}
.chart-container{background:#0f172a;border:1px solid #1e293b;border-radius:8px;padding:16px}
canvas{width:100%;height:200px}
.events-table{width:100%;border-collapse:collapse;font-size:12px}
.events-table th{text-align:left;padding:8px 12px;color:#64748b;font-weight:500;border-bottom:1px solid #1e293b}
.events-table td{padding:6px 12px;border-bottom:1px solid rgba(30,41,59,0.5)}
.events-table .level-error{color:#f87171}
.events-table .level-warn{color:#facc15}
.events-table .level-info{color:#94a3b8}
</style>
</head>
<body>
<h1>Run Report: ${run.id.slice(0, 8)}</h1>
<div class="meta">
  Status: <span class="badge badge-${run.status}">${run.status}</span>
  &nbsp;&middot;&nbsp;
  Started: ${run.started_at ? new Date(run.started_at).toLocaleString() : 'N/A'}
  &nbsp;&middot;&nbsp;
  Duration: ${formatDuration(run.started_at, run.ended_at)}
</div>

<div class="cards">
  <div class="card"><div class="card-label">Total Requests</div><div class="card-value">${run.summary?.totalRequests ?? 0}</div></div>
  <div class="card"><div class="card-label">Error Rate</div><div class="card-value">${(run.summary?.errorRate ?? 0).toFixed(2)}%</div></div>
  <div class="card"><div class="card-label">p95 Response Time</div><div class="card-value">${(run.summary?.p95 ?? 0).toFixed(1)} ms</div></div>
  <div class="card"><div class="card-label">Throughput (RPS)</div><div class="card-value">${(run.summary?.avgRps ?? 0).toFixed(1)}</div></div>
  <div class="card"><div class="card-label">Peak VUs</div><div class="card-value">${run.summary?.peakVUs ?? 0}</div></div>
  <div class="card"><div class="card-label">Failed</div><div class="card-value">${run.summary?.failed ?? 0}</div></div>
</div>

<div class="chart-section">
  <div class="chart-title">Requests per Second</div>
  <div class="chart-container"><canvas id="chart-rps"></canvas></div>
</div>
<div class="chart-section">
  <div class="chart-title">p95 Response Time (ms)</div>
  <div class="chart-container"><canvas id="chart-p95"></canvas></div>
</div>
<div class="chart-section">
  <div class="chart-title">Error Rate (%)</div>
  <div class="chart-container"><canvas id="chart-errorRate"></canvas></div>
</div>
<div class="chart-section">
  <div class="chart-title">Active Virtual Users</div>
  <div class="chart-container"><canvas id="chart-activeVUs"></canvas></div>
</div>

<h2 style="font-size:14px;font-weight:600;margin:24px 0 12px">Events</h2>
<div style="background:#0f172a;border:1px solid #1e293b;border-radius:8px;overflow:hidden">
<table class="events-table">
<thead><tr><th>Time</th><th>Level</th><th>Message</th></tr></thead>
<tbody id="events-body"></tbody>
</table>
</div>

<script>
var DATA = ${data};

function drawChart(canvasId, points, color) {
  var canvas = document.getElementById(canvasId);
  if (!canvas || points.length === 0) return;
  var ctx = canvas.getContext('2d');
  var rect = canvas.getBoundingClientRect();
  canvas.width = rect.width * (window.devicePixelRatio || 1);
  canvas.height = rect.height * (window.devicePixelRatio || 1);
  ctx.scale(window.devicePixelRatio || 1, window.devicePixelRatio || 1);
  var w = rect.width, h = rect.height;
  var pad = {l:50,r:10,t:10,b:25};
  var values = points.map(function(p){return p.value});
  var minV = Math.min.apply(null, values);
  var maxV = Math.max.apply(null, values);
  if (maxV === minV) maxV = minV + 1;
  var minT = points[0].time, maxT = points[points.length-1].time;
  if (maxT === minT) maxT = minT + 1;

  // Grid
  ctx.strokeStyle = '#1e293b';
  ctx.lineWidth = 1;
  for (var i=0;i<5;i++){
    var y = pad.t + (h-pad.t-pad.b)*i/4;
    ctx.beginPath();ctx.moveTo(pad.l,y);ctx.lineTo(w-pad.r,y);ctx.stroke();
  }

  // Y labels
  ctx.fillStyle = '#64748b';
  ctx.font = '11px Inter, sans-serif';
  ctx.textAlign = 'right';
  for (var i=0;i<5;i++){
    var val = maxV - (maxV-minV)*i/4;
    var y = pad.t + (h-pad.t-pad.b)*i/4;
    ctx.fillText(val.toFixed(1), pad.l-6, y+4);
  }

  // Line
  ctx.strokeStyle = color;
  ctx.lineWidth = 2;
  ctx.beginPath();
  for (var i=0;i<points.length;i++){
    var x = pad.l + (points[i].time - minT) / (maxT - minT) * (w - pad.l - pad.r);
    var y = pad.t + (1 - (points[i].value - minV) / (maxV - minV)) * (h - pad.t - pad.b);
    if (i===0) ctx.moveTo(x,y); else ctx.lineTo(x,y);
  }
  ctx.stroke();
}

drawChart('chart-rps', DATA.metrics.rps, '#14b8a6');
drawChart('chart-p95', DATA.metrics.p95, '#a78bfa');
drawChart('chart-errorRate', DATA.metrics.errorRate, '#f87171');
drawChart('chart-activeVUs', DATA.metrics.activeVUs, '#38bdf8');

// Events
var tbody = document.getElementById('events-body');
DATA.events.forEach(function(evt){
  var tr = document.createElement('tr');
  tr.innerHTML = '<td style="color:#64748b;white-space:nowrap;font-family:monospace">' +
    new Date(evt.timestamp).toLocaleTimeString() + '</td>' +
    '<td class="level-'+evt.level+'" style="text-transform:uppercase;font-weight:500">'+evt.level+'</td>' +
    '<td style="color:#cbd5e1">'+evt.message+'</td>';
  tbody.appendChild(tr);
});
if (DATA.events.length === 0) {
  var tr = document.createElement('tr');
  tr.innerHTML = '<td colspan="3" style="text-align:center;padding:24px;color:#475569">No events recorded</td>';
  tbody.appendChild(tr);
}
</script>
</body>
</html>`
}

function ExportDropdown({ run, metrics, events }: {
  run: Run
  metrics: {
    rps: TimeSeriesPoint[]
    p95: TimeSeriesPoint[]
    errorRate: TimeSeriesPoint[]
    activeVUs: TimeSeriesPoint[]
  }
  events: RunEvent[]
}) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  // Close on outside click
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    if (open) {
      document.addEventListener('mousedown', handleClick)
      return () => document.removeEventListener('mousedown', handleClick)
    }
  }, [open])

  const exportJSON = useCallback(async () => {
    setOpen(false)
    try {
      const res = await fetch(`/api/v1/runs/${run.id}`)
      if (res.ok) {
        const data = await res.json()
        downloadFile(
          `run-${run.id.slice(0, 8)}.json`,
          JSON.stringify(data, null, 2),
          'application/json',
        )
      }
    } catch {
      // Fallback: export what we have locally
      downloadFile(
        `run-${run.id.slice(0, 8)}.json`,
        JSON.stringify({ run, metrics, events }, null, 2),
        'application/json',
      )
    }
  }, [run, metrics, events])

  const exportCSV = useCallback(() => {
    setOpen(false)
    const csv = metricsToCSV(metrics)
    downloadFile(`run-${run.id.slice(0, 8)}.csv`, csv, 'text/csv')
  }, [run, metrics])

  const exportHTML = useCallback(() => {
    setOpen(false)
    const html = generateStandaloneHTML(run, metrics, events)
    downloadFile(`run-${run.id.slice(0, 8)}.html`, html, 'text/html')
  }, [run, metrics, events])

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen(!open)}
        className="rounded border border-slate-700 bg-slate-800 px-3 py-1 text-xs font-medium text-slate-300 hover:bg-slate-700"
      >
        Export
      </button>
      {open && (
        <div className="absolute right-0 top-full z-10 mt-1 w-40 rounded-md border border-slate-700 bg-slate-900 py-1 shadow-lg">
          <button
            onClick={exportJSON}
            className="w-full px-3 py-1.5 text-left text-xs text-slate-300 hover:bg-slate-800"
          >
            Export JSON
          </button>
          <button
            onClick={exportCSV}
            className="w-full px-3 py-1.5 text-left text-xs text-slate-300 hover:bg-slate-800"
          >
            Export CSV
          </button>
          <button
            onClick={exportHTML}
            className="w-full px-3 py-1.5 text-left text-xs text-slate-300 hover:bg-slate-800"
          >
            Export HTML
          </button>
        </div>
      )}
    </div>
  )
}

export function RunView({ runId, onBack }: RunViewProps) {
  const { run, metrics, events, wsConnected, loading, loadRun, connectWebSocket, reset } =
    useRunStore()
  const aiConfig = useAIStore((s) => s.config)
  const loadAIConfig = useAIStore((s) => s.loadConfig)
  const aiAnalysis = useAIStore((s) => s.analysis)
  const clearAnalysis = useAIStore((s) => s.clearAnalysis)

  useEffect(() => {
    loadRun(runId)
    connectWebSocket(runId)
    loadAIConfig()

    return () => {
      reset()
      clearAnalysis()
    }
  }, [runId, loadRun, connectWebSocket, reset, loadAIConfig, clearAnalysis])

  if (loading) {
    return (
      <div className="flex h-screen items-center justify-center bg-slate-950 text-slate-400">
        Loading run...
      </div>
    )
  }

  if (!run) {
    return (
      <div className="flex h-screen flex-col items-center justify-center gap-3 bg-slate-950 text-slate-400">
        <p>Run not found</p>
        <button
          onClick={onBack}
          className="rounded-md bg-slate-800 px-4 py-2 text-sm text-slate-300 hover:bg-slate-700"
        >
          Back to runs
        </button>
      </div>
    )
  }

  // Compute live summary from metrics or use run.summary if completed
  const summary = run.summary
  const lastRps = metrics.rps.length > 0 ? metrics.rps[metrics.rps.length - 1].value : 0
  const lastP95 = metrics.p95.length > 0 ? metrics.p95[metrics.p95.length - 1].value : 0
  const lastErrorRate =
    metrics.errorRate.length > 0 ? metrics.errorRate[metrics.errorRate.length - 1].value : 0
  const lastVUs =
    metrics.activeVUs.length > 0 ? metrics.activeVUs[metrics.activeVUs.length - 1].value : 0

  const totalRequests = summary?.totalRequests ?? metrics.rps.reduce((s, p) => s + p.value, 0)
  const failed = summary?.failed ?? 0
  const errorRate = summary?.errorRate ?? lastErrorRate
  const rps = summary?.avgRps ?? lastRps
  const p95 = summary?.p95 ?? lastP95
  const activeVUs = summary?.peakVUs ?? lastVUs

  const isRunning = run.status === 'running' || run.status === 'pending'
  const isTerminal = run.status === 'completed' || run.status === 'failed' || run.status === 'cancelled'

  // Live duration timer — ticks every second while running, freezes on ended_at
  const [, setTick] = useState(0)
  useEffect(() => {
    if (!isRunning) return
    const id = setInterval(() => setTick((t) => t + 1), 1000)
    return () => clearInterval(id)
  }, [isRunning])

  const liveDuration = formatDuration(run.started_at, run.ended_at)

  return (
    <div className="flex h-screen flex-col bg-slate-950 text-slate-100">
      {/* Toolbar */}
      <div className="flex h-12 items-center gap-3 border-b border-slate-800 px-4">
        <button
          onClick={onBack}
          className="rounded px-2 py-1 text-sm text-slate-400 hover:bg-slate-800 hover:text-slate-200"
        >
          &larr; Runs
        </button>
        <div className="h-4 w-px bg-slate-800" />
        <span className="text-sm font-medium">Run {run.id.slice(0, 8)}</span>
        <span
          className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${statusBadge(run.status)}`}
        >
          {run.status}
        </span>
        <span className="text-xs text-slate-500">{liveDuration}</span>
        <div className="ml-auto flex items-center gap-3">
          <AnalyseButton runId={runId} />
          <ExportDropdown run={run} metrics={metrics} events={events} />
          <span
            className={`flex items-center gap-1.5 text-xs ${
              isTerminal
                ? run.status === 'completed'
                  ? 'text-green-400'
                  : run.status === 'failed'
                    ? 'text-red-400'
                    : 'text-slate-400'
                : wsConnected
                  ? 'text-teal-400'
                  : 'text-slate-600'
            }`}
          >
            <span
              className={`inline-block h-1.5 w-1.5 rounded-full ${
                isTerminal
                  ? run.status === 'completed'
                    ? 'bg-green-400'
                    : run.status === 'failed'
                      ? 'bg-red-400'
                      : 'bg-slate-400'
                  : wsConnected
                    ? 'bg-teal-400'
                    : 'bg-slate-600'
              }`}
            />
            {isTerminal
              ? run.status === 'completed'
                ? 'Complete'
                : run.status === 'failed'
                  ? 'Failed'
                  : 'Cancelled'
              : wsConnected
                ? 'Live'
                : 'Disconnected'}
          </span>
        </div>
      </div>

      {/* Content + optional sidebar */}
      <div className="flex flex-1 overflow-hidden">
        {/* Main content */}
        <div className="flex-1 overflow-auto px-6 py-6">
          {/* Completion banner */}
          {isTerminal && <RunCompletionBanner run={run} duration={liveDuration} />}

          {/* Metric cards strip */}
          <div className="grid grid-cols-6 gap-3">
            <MetricCard label="Total Requests" value={Math.round(totalRequests).toLocaleString()} frozen={!isRunning} />
            <MetricCard label="Failed" value={Math.round(failed).toLocaleString()} alert={failed > 0} frozen={!isRunning} />
            <MetricCard
              label="Error Rate"
              value={`${errorRate.toFixed(2)}%`}
              alert={errorRate > 0}
              frozen={!isRunning}
            />
            <MetricCard label="Throughput RPS" value={rps.toFixed(1)} frozen={!isRunning} />
            <MetricCard label="p95 ms" value={p95.toFixed(1)} frozen={!isRunning} />
            <MetricCard label="Active VUs" value={Math.round(activeVUs)} frozen={!isRunning} />
          </div>

          {/* Charts 2x2 grid */}
          <div className="mt-6 grid grid-cols-2 gap-4">
            <LiveChart
              title="Requests per Second"
              data={metrics.rps}
              color="#14b8a6"
              type="line"
              yLabel="rps"
              isRunning={isRunning}
            />
            <LiveChart
              title="Response Time p95"
              data={metrics.p95}
              color="#a78bfa"
              type="line"
              yLabel="ms"
              isRunning={isRunning}
            />
            <LiveChart
              title="Error Rate"
              data={metrics.errorRate}
              color="#f87171"
              type="area"
              yLabel="%"
              isRunning={isRunning}
            />
            <LiveChart
              title="Active Virtual Users"
              data={metrics.activeVUs}
              color="#38bdf8"
              type="step"
              yLabel="VUs"
              isRunning={isRunning}
            />
          </div>

          {/* AI Insights Panel */}
          {(aiAnalysis || aiConfig.enabled) && (
            <AIInsightsPanel runId={runId} embedded={true} />
          )}

          {/* Event log */}
          <div className="mt-6">
            <h3 className="mb-3 text-sm font-medium text-slate-300">Event Log</h3>
            <div className="max-h-64 overflow-auto rounded-lg border border-slate-800 bg-slate-900">
              {events.length === 0 ? (
                <p className="px-4 py-6 text-center text-xs text-slate-600">
                  {isRunning ? 'Waiting for events...' : 'No events recorded'}
                </p>
              ) : (
                <table className="w-full text-left text-xs">
                  <thead>
                    <tr className="border-b border-slate-800">
                      <th className="px-4 py-2 font-medium text-slate-500">Time</th>
                      <th className="px-4 py-2 font-medium text-slate-500">Level</th>
                      <th className="px-4 py-2 font-medium text-slate-500">Message</th>
                    </tr>
                  </thead>
                  <tbody>
                    {events.map((evt) => {
                      const levelColor =
                        evt.level === 'error'
                          ? 'text-red-400'
                          : evt.level === 'warn'
                            ? 'text-yellow-400'
                            : 'text-slate-400'
                      return (
                        <tr key={evt.id} className="border-b border-slate-800/50">
                          <td className="whitespace-nowrap px-4 py-1.5 font-mono text-slate-500">
                            {new Date(evt.timestamp).toLocaleTimeString()}
                          </td>
                          <td className={`px-4 py-1.5 font-medium uppercase ${levelColor}`}>
                            {evt.level}
                          </td>
                          <td className="px-4 py-1.5 text-slate-300">{evt.message}</td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              )}
            </div>
          </div>
        </div>

        {/* Live control sidebar -- visible only when running */}
        {isRunning && <LiveControlPanel runId={runId} />}
      </div>
    </div>
  )
}
