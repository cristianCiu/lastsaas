import { NavLink } from 'react-router-dom';
import { MapPin, Settings, Utensils, Warehouse } from 'lucide-react';

const items = [
  { path: '/settings', label: 'Account', icon: Settings, end: true },
  { path: '/settings/restaurant', label: 'Restaurant', icon: Utensils },
  { path: '/settings/locations', label: 'Locations', icon: MapPin },
  { path: '/settings/storage-areas', label: 'Storage areas', icon: Warehouse },
];

export default function WorkspaceSettingsNav() {
  return (
    <nav aria-label="Settings sections" className="grid grid-cols-2 gap-1 rounded-xl border border-dark-800 bg-dark-900/50 p-1 sm:grid-cols-4">
      {items.map((item) => (
        <NavLink
          key={item.path}
          to={item.path}
          end={item.end}
          className={({ isActive }) => `flex min-w-0 items-center justify-center gap-2 rounded-lg px-3 py-2 text-sm font-medium transition-colors ${
            isActive ? 'bg-dark-700 text-white' : 'text-dark-400 hover:bg-dark-800/60 hover:text-dark-200'
          }`}
        >
          <item.icon className="h-4 w-4 shrink-0" />
          <span className="truncate">{item.label}</span>
        </NavLink>
      ))}
    </nav>
  );
}
