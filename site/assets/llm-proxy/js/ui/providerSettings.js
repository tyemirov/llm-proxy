// @ts-check

import {
  AUTH_STATES,
  COPY,
  NOTICE_KINDS,
  PROVIDER_KEY_VERIFICATION_ERRORS,
} from "../constants.js?v=20260811c131";
import { saveProviderConnection as requestSaveProviderConnection } from "../core/backendClient.js?v=20260811c131";
import {
  isAbortError,
  profileFailureMessage,
} from "../core/managementProfile.js?v=20260811c131";

const EMPTY_STRING = "";

/** @typedef {ReturnType<typeof import("./managementApplicationState.js").createManagementApplicationState>} ManagementApplicationState */
/** @typedef {ManagementApplicationState & {
 *   selectedProvider: import("../types.d.js").ProviderProfile | null,
 *   selectedProviderID: string,
 *   abortProviderKeyVerification: () => void,
 *   applyProfile: (profile: import("../types.d.js").ManagementTenantProfile, preserveProviderEditor?: boolean, preserveRoutingDefaults?: boolean) => void,
 *   enqueueProfileMutation: (appVersion: number, mutation: () => Promise<boolean>) => Promise<boolean | null>,
 *   setSettingsNotice: (kind: string, message: string) => void
 * }} ProviderSettingsHost */

/**
 * @template {object} Responsibility
 * @param {Responsibility & ThisType<ProviderSettingsHost & Responsibility>} responsibility
 * @returns {Responsibility}
 */
function providerSettingsResponsibility(responsibility) {
  return responsibility;
}

/** Create serialized provider-settings persistence and candidate-key verification. */
export function createProviderSettingsResponsibility() {
  return providerSettingsResponsibility({
    async autosaveSelectedProvider() {
      if (this.providerAutosavePromise) {
        return this.providerAutosavePromise;
      }
      if (!this.providerEditorSession.dirty) {
        return true;
      }
      const autosavePromise = this.persistSelectedProviderChanges();
      this.providerAutosavePromise = autosavePromise;
      this.providerAutosavePending = true;
      try {
        return await autosavePromise;
      } finally {
        if (this.providerAutosavePromise === autosavePromise) {
          this.providerAutosavePromise = null;
          this.providerAutosavePending = false;
        }
      }
    },

    async persistSelectedProviderChanges() {
      while (this.providerEditorSession.dirty) {
        const provider = this.selectedProvider;
        if (!provider) {
          return false;
        }
        const editorSession = this.providerEditorSession;
        const fields = providerConnectionFields(provider, editorSession);
        if (!provider.configured && !requiredProviderFieldsHaveValues(provider, fields)) {
          editorSession.dirty = false;
          return true;
        }
        const providerID = provider.id;
        const revealVersion = editorSession.revealVersion;
        const editVersion = editorSession.editVersion;
        const appVersion = this.appVersion;
        const tenantID = this.settingsTenantID;
        const lifetimeController = this.tenantLifetimeController;
        if (!lifetimeController) {
          return false;
        }
        const verifiesConnectionFields = Object.values(editorSession.fieldDirty).some(Boolean);
        const verifiesCandidate = verifiesConnectionFields || editorSession.textModel !== provider.text_model;
        let requestSignal = lifetimeController.signal;
        /** @type {AbortController | null} */
        let verificationController = null;
        /** @type {() => void} */
        let detachLifetimeAbort = () => {};
        if (verifiesCandidate) {
          this.abortProviderKeyVerification();
          const candidateVerificationController = new AbortController();
          verificationController = candidateVerificationController;
          this.providerKeyVerificationController = candidateVerificationController;
          this.providerKeyVerificationPending = verifiesConnectionFields;
          this.providerKeyVerificationFailure = EMPTY_STRING;
          const abortForTenantLifetime = () => {
            candidateVerificationController.abort();
          };
          if (lifetimeController.signal.aborted) {
            abortForTenantLifetime();
          } else {
            lifetimeController.signal.addEventListener("abort", abortForTenantLifetime, { once: true });
            detachLifetimeAbort = () => {
              lifetimeController.signal.removeEventListener("abort", abortForTenantLifetime);
            };
          }
          requestSignal = candidateVerificationController.signal;
        }
        editorSession.dirty = false;
        try {
          const profileApplied = await this.enqueueProfileMutation(appVersion, async () => {
            const updatedProfile = await requestSaveProviderConnection(
              tenantID,
              providerID,
              fields,
              editorSession.textModel,
              editorSession.systemPrompt,
              requestSignal,
            );
            if (!this.canApplyProviderAutosave(providerID, revealVersion, appVersion)) {
              return false;
            }
            const preserveProviderEditor = this.providerEditorSession.editVersion !== editVersion;
            this.applyProfile(
              updatedProfile,
              preserveProviderEditor,
              this.routingDefaultsDirty || this.routingDefaultsAutosavePending,
            );
            if (!preserveProviderEditor) {
              this.setSettingsNotice(
                NOTICE_KINDS.SUCCESS,
                verifiesCandidate ? COPY.providerKeyVerified : COPY.providerSettingsSaved,
              );
            }
            return true;
          });
          if (!profileApplied) {
            return false;
          }
        } catch (requestError) {
          if (this.canApplyProviderAutosave(providerID, revealVersion, appVersion)) {
            this.providerEditorSession.dirty = true;
            if (!isAbortError(requestError)) {
              const verificationError = verifiesCandidate
                ? providerKeyVerificationError(requestError)
                : null;
              const failureMessage = verificationError
                ? providerKeyVerificationFailureMessage(verificationError, provider.configured)
                : profileFailureMessage(requestError);
              this.providerKeyVerificationFailure = verificationError ? failureMessage : EMPTY_STRING;
              this.setSettingsNotice(NOTICE_KINDS.ERROR, failureMessage);
            }
          }
          return false;
        } finally {
          detachLifetimeAbort();
          if (
            verificationController &&
            this.providerKeyVerificationController === verificationController
          ) {
            this.providerKeyVerificationController = null;
            this.providerKeyVerificationPending = false;
          }
        }
      }
      return true;
    },

    /**
     * @param {string} providerID
     * @param {number} revealVersion
     * @param {number} appVersion
     * @returns {boolean}
     */
    canApplyProviderAutosave(providerID, revealVersion, appVersion) {
      return (
        this.settingsOpen &&
        this.authState === AUTH_STATES.AUTHENTICATED &&
        this.appVersion === appVersion &&
        this.selectedProviderID === providerID &&
        this.providerEditorSession.revealVersion === revealVersion
      );
    },
  });
}

/**
 * @param {import("../types.d.js").ProviderProfile} provider
 * @param {import("../types.d.js").ProviderEditorSession} editorSession
 * @returns {Record<string, string>}
 */
function providerConnectionFields(provider, editorSession) {
  return Object.fromEntries(provider.fields.map((field) => [
    field.id,
    field.secret && !editorSession.fieldDirty[field.id]
      ? EMPTY_STRING
      : String(editorSession.fieldInputs[field.id] ?? EMPTY_STRING).trim(),
  ]));
}

/**
 * @param {import("../types.d.js").ProviderProfile} provider
 * @param {Record<string, string>} fields
 */
function requiredProviderFieldsHaveValues(provider, fields) {
  return provider.fields.every((field) => !field.required || fields[field.id] !== EMPTY_STRING);
}

/** @param {unknown} requestError */
function providerKeyVerificationError(requestError) {
  if (!(requestError instanceof Error)) {
    return null;
  }
  const errorCode = requestError.message.trim();
  if (!Object.values(PROVIDER_KEY_VERIFICATION_ERRORS).some((knownError) => knownError === errorCode)) {
    return null;
  }
  return /** @type {import("../types.d.js").ProviderKeyVerificationError} */ (errorCode);
}

/**
 * @param {import("../types.d.js").ProviderKeyVerificationError} verificationError
 * @param {boolean} previousKeyActive
 */
function providerKeyVerificationFailureMessage(verificationError, previousKeyActive) {
  switch (verificationError) {
    case PROVIDER_KEY_VERIFICATION_ERRORS.REJECTED:
      return previousKeyActive ? COPY.providerKeyRejectedPreviousActive : COPY.providerKeyRejectedUnsaved;
    case PROVIDER_KEY_VERIFICATION_ERRORS.RATE_LIMITED:
      return previousKeyActive ? COPY.providerKeyRateLimitedPreviousActive : COPY.providerKeyRateLimitedUnsaved;
    case PROVIDER_KEY_VERIFICATION_ERRORS.TIMED_OUT:
      return previousKeyActive ? COPY.providerKeyTimedOutPreviousActive : COPY.providerKeyTimedOutUnsaved;
    case PROVIDER_KEY_VERIFICATION_ERRORS.UNAVAILABLE:
      return previousKeyActive ? COPY.providerKeyUnavailablePreviousActive : COPY.providerKeyUnavailableUnsaved;
    default:
      return COPY.requestFailed;
  }
}
