import React from 'react';
import ReactDOM from 'react-dom/client';
import './colors.css';
import './index.css';
import App from './App';
import ErrorBoundary from './components/ErrorBoundary';
import ServiceWorkerUpdatePrompt from './components/ServiceWorkerUpdatePrompt';
import reportWebVitals from './reportWebVitals';
import * as serviceWorkerRegistration from './serviceWorkerRegistration';
import { notifyUpdateAvailable } from './serviceWorkerUpdates';
import './i18n/config';
import { AppThemeProvider } from './AppThemeProvider';
import { AnnouncerProvider } from './context/AnnouncerContext';
import { SnackbarProvider } from './context/SnackbarContext';
import { DateFormatProvider } from './DateFormatProvider';

const logError = (error: Error, errorInfo: React.ErrorInfo) => {
  console.error('Application Error:', error);
  console.error('Error Info:', errorInfo);
};

const root = ReactDOM.createRoot(document.getElementById('root') as HTMLElement);

root.render(
  <React.StrictMode>
    <AppThemeProvider>
      <DateFormatProvider>
        <SnackbarProvider>
          <AnnouncerProvider>
            <ErrorBoundary name="Application" onError={logError} showDetails={import.meta.env.DEV}>
              <App />
              <ServiceWorkerUpdatePrompt />
            </ErrorBoundary>
          </AnnouncerProvider>
        </SnackbarProvider>
      </DateFormatProvider>
    </AppThemeProvider>
  </React.StrictMode>,
);

// The service worker is REGISTERED, not unregistered (CRA scaffolds the
// opposite and it stayed that way until 2026-08-06).
//
// This is not primarily about offline support: Web Push (N9) delivers to a
// service worker, and a PushSubscription is owned by the registration. While
// this called unregister(), every page load tore down the registration and
// took any push subscription with it -- so enabling browser notifications
// appeared to work and then silently stopped, with the server's stored
// subscription going stale and being pruned on the next 404/410.
//
// The cost of registering is that the app is now served cache-first, so a
// deployed update is not picked up until the new worker takes over.
// ServiceWorkerUpdatePrompt turns that into an explicit "reload to update"
// notice rather than a user sitting on a stale bundle indefinitely.
//
// register() is a no-op outside production builds and on insecure origins
// (browsers refuse to register a service worker over plain HTTP), which is why
// browser push needs HTTPS or localhost.
serviceWorkerRegistration.register({ onUpdate: notifyUpdateAvailable });

// If you want to start measuring performance in your app, pass a function
// to log results (for example: reportWebVitals(console.log))
// or send to an analytics endpoint. Learn more: https://bit.ly/CRA-vitals
reportWebVitals();
