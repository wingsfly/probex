import { Routes, Route, NavLink } from 'react-router-dom'
import Dashboard from './pages/Dashboard'
import Tasks from './pages/Tasks'
import Probes from './pages/Probes'
import Results from './pages/Results'
import Reports from './pages/Reports'
import Alerts from './pages/Alerts'
import Agents from './pages/Agents'

const navItems = [
  { path: '/', label: 'Dashboard' },
  { path: '/tasks', label: 'Tasks' },
  { path: '/probes', label: 'Probes' },
  { path: '/results', label: 'Results' },
  { path: '/reports', label: 'Reports' },
  { path: '/alerts', label: 'Alerts' },
  { path: '/agents', label: 'Agents' },
]

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
          {navItems.map(({ path, label }) => (
            <NavLink
              key={path}
              to={path}
              end={path === '/'}
              style={({ isActive }) => ({
                display: 'block', padding: '0.6rem 1.5rem', fontSize: '0.875rem',
                color: isActive ? '#fff' : '#94a3b8',
                background: isActive ? '#334155' : 'transparent',
                textDecoration: 'none', borderLeft: isActive ? '3px solid #3b82f6' : '3px solid transparent',
              })}
            >
              {label}
            </NavLink>
          ))}
        </nav>
      </aside>

      <main style={{ flex: 1, padding: '1.5rem 2rem', maxWidth: 1200 }}>
        <Routes>
          <Route path="/" element={<Dashboard />} />
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
