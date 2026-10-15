import { useState, useEffect } from 'react';
import {
  getResolutions,
  getBrowsers,
  getTransports,
  getDefaultTransport,
} from '../lib/config';

export function useConfig() {
  const [ready, setReady] = useState(false);

  useEffect(() => {
    setReady(true);
  }, []);

  const resolutions = ready ? getResolutions() : [];
  const browsers = ready ? getBrowsers() : [];
  const transports = ready ? getTransports() : [];
  const defaultTransport = ready ? getDefaultTransport() : 'isola';

  return {
    resolutionVisible: resolutions.length > 0,
    resolutions,
    browserVisible: browsers.length > 0,
    browsers,
    transportVisible: transports.length > 0,
    transports,
    defaultTransport,
  };
}
