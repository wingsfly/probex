import { Routes, Route, NavLink } from 'react-router-dom'
import Overview from './pages/Overview'
import Dashboard from './pages/Dashboard'
import Nodes from './pages/Nodes'
import Tasks from './pages/Tasks'
import Probes from './pages/Probes'
import Results from './pages/Results'
import Heatmap from './pages/Heatmap'
import Reports from './pages/Reports'
import Alerts from './pages/Alerts'
import Agents from './pages/Agents'

const navItems = [
  { path: '/', label: 'Overview', section: 'monitor' },
  { path: '/dashboard', label: 'Dashboard', section: 'monitor' },
  { path: '/nodes', label: 'Nodes', section: 'monitor' },
  { path: '/heatmap', label: 'Heatmap', section: 'monitor' },
  { path: '/tasks', label: 'Tasks', section: 'config' },
  { path: '/probes', label: 'Probes', section: 'config' },
  { path: '/results', label: 'Results', section: 'data' },
  { path: '/reports', label: 'Reports', section: 'data' },
  { path: '/alerts', label: 'Alerts', section: 'data' },
]

const sections: Record<string, string> = {
  monitor: 'MONITORING',
  config: 'CONFIGURATION',
  data: 'DATA & ALERTS',
}

export default function App() {
  return (
    <div style={{ display: 'flex', minHeight: '100vh', background: '#f9fafb' }}>
      <aside style={{
        width: 220, background: '#1e293b', color: '#fff', padding: '1.5rem 0',
        display: 'flex', flexDirection: 'column', flexShrink: 0,
      }}>
        <div style={{ padding: '0 1.5rem', marginBottom: '2rem' }}>
          <h1 style={{ fontSize: '1.25rem', fontWeight: 700, letterSpacing: '-0.02em' }}>
            ProbeX
          </h1>
          <p style={{ fontSize: '0.7rem', color: '#94a3b8', marginTop: 2 }}>Network Quality Monitor</p>
        </div>
        <nav>
          {Object.entries(sections).map(([key, label]) => (
            <div key={key}>
              <div style={{ padding: '0.5rem 1.5rem 0.25rem', fontSize: '0.6rem', fontWeight: 600, color: '#475569', letterSpacing: '0.05em' }}>
                {label}
              </div>
              {navItems.filter(n => n.section === key).map(({ path, label: navLabel }) => (
                <NavLink
                  key={path}
                  to={path}
                  end={path === '/'}
                  style={({ isActive }) => ({
                    display: 'block', padding: '0.5rem 1.5rem', fontSize: '0.85rem',
                    color: isActive ? '#fff' : '#94a3b8',
                    background: isActive ? '#334155' : 'transparent',
                    textDecoration: 'none', borderLeft: isActive ? '3px solid #3b82f6' : '3px solid transparent',
                  })}
                >
                  {navLabel}
                </NavLink>
              ))}
            </div>
          ))}
        </nav>
      </aside>

      <main style={{ flex: 1, padding: '1.5rem 2rem', maxWidth: 1400 }}>
        <Routes>
          <Route path="/" element={<Overview />} />
          <Route path="/dashboard" element={<Dashboard />} />
          <Route path="/nodes" element={<Nodes />} />
          <Route path="/heatmap" element={<Heatmap />} />
          <Route path="/tasks" element={<Tasks />} />
          <Route path="/probes" element={<Probes />} />
          <Route path="/results" element={<Results />} />
          <Route path="/reports" element={<Reports />} />
          <Route path="/alerts" element={<Alerts />} />
          <Route path="/agents" element={<Agents />} />
        </Routes>
      </main>
    </div>
  )
}
