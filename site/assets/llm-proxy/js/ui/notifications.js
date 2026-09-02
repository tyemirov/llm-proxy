// @ts-check

import {
  NOTICE_AUTO_DISMISS_MILLISECONDS,
  NOTICE_KINDS,
  NOTICE_SURFACES,
} from "../constants.js?v=20260902a237";

const EMPTY_STRING = "";

/** @typedef {ReturnType<typeof import("./managementApplicationState.js").createManagementApplicationState>} ManagementApplicationState */

/**
 * @template {object} Responsibility
 * @param {Responsibility & ThisType<ManagementApplicationState & Responsibility>} responsibility
 * @returns {Responsibility}
 */
function notificationResponsibility(responsibility) {
  return responsibility;
}

/** Create notification behavior for page and Settings surfaces. */
export function createNotificationsResponsibility() {
  return notificationResponsibility({
    /**
     * @param {string} kind
     * @param {string} message
     */
    setPageNotice(kind, message) {
      this.setNotice(kind, message, NOTICE_SURFACES.HEADER);
    },

    /**
     * @param {string} kind
     * @param {string} message
     */
    setSettingsNotice(kind, message) {
      this.setNotice(kind, message, NOTICE_SURFACES.SETTINGS);
    },

    /**
     * @param {string} kind
     * @param {string} message
     * @param {string} surface
     */
    setNotice(kind, message, surface) {
      this.clearNotice();
      this.notice = { kind, message, surface };
      if (message === EMPTY_STRING) {
        return;
      }
      const noticeVersion = this.noticeVersion + 1;
      this.noticeVersion = noticeVersion;
      this.noticeDismissTimerID = window.setTimeout(() => {
        if (this.noticeVersion !== noticeVersion) {
          return;
        }
        this.clearNotice();
      }, NOTICE_AUTO_DISMISS_MILLISECONDS);
    },

    clearNotice() {
      this.noticeVersion += 1;
      if (this.noticeDismissTimerID !== null) {
        window.clearTimeout(this.noticeDismissTimerID);
        this.noticeDismissTimerID = null;
      }
      this.notice = {
        kind: NOTICE_KINDS.INFO,
        message: EMPTY_STRING,
        surface: NOTICE_SURFACES.HEADER,
      };
    },
  });
}
