// @ts-check

(() => {
  const GOOGLE_ANALYTICS_MEASUREMENT_ID = "G-Z780618FPW";
  const analyticsWindow = /** @type {Window & {dataLayer?: unknown[][], gtag?: (...args: unknown[]) => void}} */ (window);
  const dataLayer = analyticsWindow.dataLayer || [];

  analyticsWindow.dataLayer = dataLayer;
  analyticsWindow.gtag = function gtag(...args) {
    dataLayer.push(args);
  };
  analyticsWindow.gtag("js", new Date());
  analyticsWindow.gtag("config", GOOGLE_ANALYTICS_MEASUREMENT_ID);

  const googleTag = document.createElement("script");
  googleTag.async = true;
  googleTag.src = `https://www.googletagmanager.com/gtag/js?id=${encodeURIComponent(GOOGLE_ANALYTICS_MEASUREMENT_ID)}`;
  document.head.append(googleTag);
})();
