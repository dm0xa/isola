import type { ComponentType } from 'react';

export interface PluginInfo {
  name: string;
  description: string;
  version: string;
}

export interface ToolbarButtonProps {
  sessionId: string;
  viewOnly: boolean;
}

export interface ToolbarButton {
  id: string;
  icon: string; // lucide icon name
  label: string;
  order: number;
  component: ComponentType<ToolbarButtonProps>;
}

export interface SidePanelProps {
  sessionId: string;
}

export interface SidePanel {
  id: string;
  title: string;
  component: ComponentType<SidePanelProps>;
}

export interface PluginRoute {
  path: string;
  element: React.ReactElement;
}

export interface FrontendPlugin {
  info: PluginInfo;
  /** Optional React context provider that wraps the session viewer for shared plugin state. */
  provider?: ComponentType<{ sessionId: string; viewOnly: boolean; children: React.ReactNode }>;
  toolbarButtons?: ToolbarButton[];
  sidePanels?: SidePanel[];
  routes?: PluginRoute[];
}
