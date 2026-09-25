import type { Metric } from 'web-vitals';

type ReportHandler = (metric: Metric) => void;

const reportWebVitals = (onPerfEntry?: ReportHandler) => {
  if (onPerfEntry && onPerfEntry instanceof Function) {
    void import('web-vitals')
      .then(({ onCLS, onFCP, onLCP, onTTFB }) => {
        onCLS(onPerfEntry);
        onFCP(onPerfEntry);
        onLCP(onPerfEntry);
        onTTFB(onPerfEntry);
      })
      .catch((err: unknown) => {
        console.error('Failed to load web-vitals:', err);
      });
  }
};

export default reportWebVitals;
