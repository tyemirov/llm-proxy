// @ts-check

import {
  CAPABILITY_CATALOG_COPY,
  CAPABILITY_CATALOG_SORT_DIRECTIONS,
  CAPABILITY_CATALOG_SORTS,
} from "../constants.js";

const SELECTORS = Object.freeze({
  BODY: "[data-catalog-body]",
  CAPABILITY: "[data-catalog-capability]",
  CAPABILITY_ACTION: "[data-catalog-capability-action]",
  EMPTY: "[data-catalog-empty]",
  FILTER_PANEL: "[data-catalog-filter-panel]",
  FORM: "[data-catalog-toolbar]",
  RESULT_COUNT: "[data-catalog-result-count]",
  ROW: "[data-catalog-row]",
  SEARCH: "[data-catalog-search]",
  SEARCH_SUBMIT: "[data-catalog-search-submit]",
  SORT: "[data-catalog-sort]",
  SORT_HEADER: "[data-catalog-sort-header]",
});

/**
 * @typedef {{
 *   element: HTMLTableRowElement,
 *   publisher: string,
 *   model: string,
 *   capabilities: Set<string>,
 *   capabilityCount: number,
 *   searchText: string,
 * }} CapabilityCatalogRow
 */

/**
 * @typedef {(typeof CAPABILITY_CATALOG_SORTS)[keyof typeof CAPABILITY_CATALOG_SORTS]} CapabilityCatalogSort
 */

/**
 * @typedef {(typeof CAPABILITY_CATALOG_SORT_DIRECTIONS)[keyof typeof CAPABILITY_CATALOG_SORT_DIRECTIONS]} CapabilityCatalogSortDirection
 */

class CapabilityCatalogElement extends HTMLElement {
  constructor() {
    super();
    /** @type {HTMLFormElement | null} */
    this.form = null;
    /** @type {HTMLInputElement | null} */
    this.searchInput = null;
    /** @type {HTMLButtonElement | null} */
    this.searchSubmit = null;
    /** @type {HTMLElement | null} */
    this.filterPanel = null;
    /** @type {HTMLButtonElement[]} */
    this.sortButtons = [];
    /** @type {HTMLTableSectionElement | null} */
    this.tableBody = null;
    /** @type {HTMLOutputElement | null} */
    this.resultCount = null;
    /** @type {HTMLElement | null} */
    this.emptyState = null;
    /** @type {CapabilityCatalogRow[]} */
    this.rows = [];
    /** @type {CapabilityCatalogSort} */
    this.sortColumn = CAPABILITY_CATALOG_SORTS.PUBLISHER;
    /** @type {CapabilityCatalogSortDirection} */
    this.sortDirection = CAPABILITY_CATALOG_SORT_DIRECTIONS.ASCENDING;
    this.updateFrameIdentifier = 0;
  }

  connectedCallback() {
    if (this.dataset.enhanced === "true") {
      return;
    }
    this.form = requiredElement(this, SELECTORS.FORM, HTMLFormElement);
    this.searchInput = requiredElement(this, SELECTORS.SEARCH, HTMLInputElement);
    this.searchSubmit = requiredElement(this, SELECTORS.SEARCH_SUBMIT, HTMLButtonElement);
    this.filterPanel = requiredElement(this, SELECTORS.FILTER_PANEL, HTMLElement);
    this.tableBody = requiredElement(this, SELECTORS.BODY, HTMLTableSectionElement);
    this.resultCount = requiredElement(this, SELECTORS.RESULT_COUNT, HTMLOutputElement);
    this.emptyState = requiredElement(this, SELECTORS.EMPTY, HTMLElement);
    this.rows = [...this.querySelectorAll(SELECTORS.ROW)].map(createCapabilityCatalogRow);
    this.sortButtons = [...this.querySelectorAll(SELECTORS.SORT)].map((element) => {
      if (!(element instanceof HTMLButtonElement)) {
        throw new Error("capability_catalog_sort_control_invalid");
      }
      return element;
    });
    if (this.sortButtons.length !== Object.keys(CAPABILITY_CATALOG_SORTS).length) {
      throw new Error(`capability_catalog_sort_controls_invalid: count=${this.sortButtons.length}`);
    }

    this.form.addEventListener("input", (event) => {
      if (event.target === this.searchInput) {
        this.showFilterPanel();
      }
      this.scheduleUpdate();
    });
    this.form.addEventListener("change", () => this.scheduleUpdate());
    this.form.addEventListener("submit", (event) => {
      event.preventDefault();
      this.showFilterPanel();
      this.scheduleUpdate();
    });
    this.form.addEventListener("reset", () => this.resetCatalog());
    this.searchSubmit.addEventListener("click", () => this.toggleFilterPanel());
    this.searchInput.addEventListener("focus", () => this.showFilterPanel());
    this.searchInput.addEventListener("search", () => {
      this.showFilterPanel();
      this.scheduleUpdate();
    });
    this.searchInput.addEventListener("keydown", (event) => {
      if (event.key === "Enter") {
        event.preventDefault();
        this.showFilterPanel();
        this.scheduleUpdate();
        return;
      }
      if (event.key !== "Escape") {
        return;
      }
      event.preventDefault();
      this.hideFilterPanel();
    });
    for (const sortButton of this.sortButtons) {
      sortButton.disabled = false;
      sortButton.addEventListener("click", () => this.activateSort(sortButton));
    }
    for (const action of this.querySelectorAll(SELECTORS.CAPABILITY_ACTION)) {
      const capabilityAction = /** @type {HTMLButtonElement} */ (action);
      capabilityAction.disabled = false;
      capabilityAction.addEventListener("click", () => this.activateCapabilityFilter(capabilityAction));
    }
    this.dataset.enhanced = "true";
    this.updateSortControls();
    this.scheduleUpdate();
  }

  showFilterPanel() {
    if (!this.filterPanel || !this.searchSubmit) {
      throw new Error("capability_catalog_not_connected");
    }
    this.filterPanel.hidden = false;
    this.searchSubmit.setAttribute("aria-expanded", "true");
  }

  hideFilterPanel() {
    if (!this.filterPanel || !this.searchSubmit) {
      throw new Error("capability_catalog_not_connected");
    }
    this.filterPanel.hidden = true;
    this.searchSubmit.setAttribute("aria-expanded", "false");
  }

  toggleFilterPanel() {
    if (!this.filterPanel) {
      throw new Error("capability_catalog_not_connected");
    }
    if (this.filterPanel.hidden) {
      this.showFilterPanel();
      return;
    }
    this.hideFilterPanel();
  }

  resetCatalog() {
    this.sortColumn = CAPABILITY_CATALOG_SORTS.PUBLISHER;
    this.sortDirection = CAPABILITY_CATALOG_SORT_DIRECTIONS.ASCENDING;
    this.updateSortControls();
    this.scheduleUpdate();
  }

  /** @param {HTMLButtonElement} sortButton */
  activateSort(sortButton) {
    const sortColumn = requiredDatasetValue(sortButton, "catalogSort");
    if (!isCapabilityCatalogSort(sortColumn)) {
      throw new Error(`capability_catalog_sort_invalid: sort=${sortColumn}`);
    }
    if (sortColumn === this.sortColumn) {
      this.sortDirection = this.sortDirection === CAPABILITY_CATALOG_SORT_DIRECTIONS.ASCENDING
        ? CAPABILITY_CATALOG_SORT_DIRECTIONS.DESCENDING
        : CAPABILITY_CATALOG_SORT_DIRECTIONS.ASCENDING;
    } else {
      this.sortColumn = sortColumn;
      this.sortDirection = sortColumn === CAPABILITY_CATALOG_SORTS.CAPABILITIES
        ? CAPABILITY_CATALOG_SORT_DIRECTIONS.DESCENDING
        : CAPABILITY_CATALOG_SORT_DIRECTIONS.ASCENDING;
    }
    this.updateSortControls();
    this.scheduleUpdate();
  }

  updateSortControls() {
    for (const sortButton of this.sortButtons) {
      const sortColumn = requiredDatasetValue(sortButton, "catalogSort");
      const sortLabel = requiredDatasetValue(sortButton, "sortLabel");
      const sortHeader = sortButton.closest(SELECTORS.SORT_HEADER);
      if (!(sortHeader instanceof HTMLTableCellElement)) {
        throw new Error(`capability_catalog_sort_header_missing: sort=${sortColumn}`);
      }
      const active = sortColumn === this.sortColumn;
      if (active) {
        sortHeader.setAttribute("aria-sort", this.sortDirection);
        sortButton.dataset.sortDirection = this.sortDirection;
      } else {
        sortHeader.removeAttribute("aria-sort");
        delete sortButton.dataset.sortDirection;
      }
      const nextDirection = active
        ? (this.sortDirection === CAPABILITY_CATALOG_SORT_DIRECTIONS.ASCENDING
          ? CAPABILITY_CATALOG_SORT_DIRECTIONS.DESCENDING
          : CAPABILITY_CATALOG_SORT_DIRECTIONS.ASCENDING)
        : (sortColumn === CAPABILITY_CATALOG_SORTS.CAPABILITIES
          ? CAPABILITY_CATALOG_SORT_DIRECTIONS.DESCENDING
          : CAPABILITY_CATALOG_SORT_DIRECTIONS.ASCENDING);
      sortButton.setAttribute("aria-label", `${CAPABILITY_CATALOG_COPY.SORT_BY} ${sortLabel} ${nextDirection}`);
    }
  }

  /** @param {HTMLButtonElement} action */
  activateCapabilityFilter(action) {
    const capabilityIdentifier = action.dataset.catalogCapabilityAction;
    const matchingFilter = [...this.querySelectorAll(SELECTORS.CAPABILITY)].find(
      (candidate) => /** @type {HTMLInputElement} */ (candidate).value === capabilityIdentifier,
    );
    if (!matchingFilter) {
      throw new Error(`capability_catalog_filter_missing: capability=${capabilityIdentifier}`);
    }
    /** @type {HTMLInputElement} */ (matchingFilter).checked = true;
    this.showFilterPanel();
    this.scheduleUpdate();
  }

  scheduleUpdate() {
    if (this.updateFrameIdentifier !== 0) {
      return;
    }
    this.updateFrameIdentifier = window.requestAnimationFrame(() => {
      this.updateFrameIdentifier = 0;
      this.applyCurrentControls();
    });
  }

  applyCurrentControls() {
    if (!this.searchInput || !this.tableBody || !this.resultCount || !this.emptyState) {
      throw new Error("capability_catalog_not_connected");
    }
    const searchTerms = capabilityCatalogSearchTerms(this.searchInput.value);
    const selectedCapabilities = [...this.querySelectorAll(SELECTORS.CAPABILITY)]
      .map((candidate) => /** @type {HTMLInputElement} */ (candidate))
      .filter((candidate) => candidate.checked)
      .map((candidate) => candidate.value);
    const visibleRows = this.rows.filter(
      (row) =>
        searchTerms.every((searchTerm) => row.searchText.includes(searchTerm)) &&
        selectedCapabilities.every((capability) => row.capabilities.has(capability)),
    );
    const sortedRows = [...visibleRows].sort(sortCapabilityCatalogRows(this.sortColumn, this.sortDirection));
    const visibleElements = new Set(sortedRows.map((row) => row.element));
    for (const row of this.rows) {
      row.element.hidden = !visibleElements.has(row.element);
    }
    for (const row of sortedRows) {
      this.tableBody.append(row.element);
    }
    this.emptyState.hidden = sortedRows.length !== 0;
    const modelLabel = sortedRows.length === 1 ? CAPABILITY_CATALOG_COPY.MODEL : CAPABILITY_CATALOG_COPY.MODELS;
    this.resultCount.textContent = `${sortedRows.length} ${CAPABILITY_CATALOG_COPY.RESULT_SEPARATOR} ${this.rows.length} ${modelLabel}`;
  }
}

/**
 * @param {Element} rowElement
 * @returns {CapabilityCatalogRow}
 */
function createCapabilityCatalogRow(rowElement) {
  const tableRow = /** @type {HTMLTableRowElement} */ (rowElement);
  const publisher = requiredDatasetValue(tableRow, "publisher");
  const model = requiredDatasetValue(tableRow, "model");
  const capabilities = new Set(requiredDatasetValue(tableRow, "capabilities").split(" "));
  const capabilityCount = Number(requiredDatasetValue(tableRow, "capabilityCount"));
  if (!Number.isSafeInteger(capabilityCount) || capabilityCount < 1) {
    throw new Error(`capability_catalog_capability_count_invalid: publisher=${publisher} model=${model}`);
  }
  return {
    element: tableRow,
    publisher,
    model,
    capabilities,
    capabilityCount,
    searchText: requiredDatasetValue(tableRow, "catalogSearchText").toLocaleLowerCase(),
  };
}

/**
 * @param {string} rawSearch
 * @returns {string[]}
 */
function capabilityCatalogSearchTerms(rawSearch) {
  return rawSearch.trim().toLocaleLowerCase().split(/\s+/u).filter(Boolean);
}

/**
 * @param {CapabilityCatalogSort} sort
 * @param {CapabilityCatalogSortDirection} direction
 * @returns {(firstRow: CapabilityCatalogRow, secondRow: CapabilityCatalogRow) => number}
 */
function sortCapabilityCatalogRows(sort, direction) {
  if (!isCapabilityCatalogSort(sort)) {
    throw new Error(`capability_catalog_sort_invalid: sort=${sort}`);
  }
  if (!Object.values(CAPABILITY_CATALOG_SORT_DIRECTIONS).includes(direction)) {
    throw new Error(`capability_catalog_sort_direction_invalid: direction=${direction}`);
  }
  const directionMultiplier = direction === CAPABILITY_CATALOG_SORT_DIRECTIONS.ASCENDING ? 1 : -1;
  if (sort === CAPABILITY_CATALOG_SORTS.MODEL) {
    return (firstRow, secondRow) => directionMultiplier * (
      compareText(firstRow.model, secondRow.model) || compareText(firstRow.publisher, secondRow.publisher)
    );
  }
  if (sort === CAPABILITY_CATALOG_SORTS.CAPABILITIES) {
    return (firstRow, secondRow) =>
      directionMultiplier * (firstRow.capabilityCount - secondRow.capabilityCount) || comparePublisherAndModel(firstRow, secondRow);
  }
  return (firstRow, secondRow) => directionMultiplier * comparePublisherAndModel(firstRow, secondRow);
}

/**
 * @param {string} value
 * @returns {value is CapabilityCatalogSort}
 */
function isCapabilityCatalogSort(value) {
  return value === CAPABILITY_CATALOG_SORTS.PUBLISHER ||
    value === CAPABILITY_CATALOG_SORTS.MODEL ||
    value === CAPABILITY_CATALOG_SORTS.CAPABILITIES;
}

/**
 * @param {CapabilityCatalogRow} firstRow
 * @param {CapabilityCatalogRow} secondRow
 * @returns {number}
 */
function comparePublisherAndModel(firstRow, secondRow) {
  return compareText(firstRow.publisher, secondRow.publisher) || compareText(firstRow.model, secondRow.model);
}

/**
 * @param {string} firstValue
 * @param {string} secondValue
 * @returns {number}
 */
function compareText(firstValue, secondValue) {
  return firstValue.localeCompare(secondValue, undefined, { sensitivity: "base" });
}

/**
 * @template {Element} ElementType
 * @param {ParentNode} root
 * @param {string} selector
 * @param {{new(): ElementType}} expectedType
 * @returns {ElementType}
 */
function requiredElement(root, selector, expectedType) {
  const element = root.querySelector(selector);
  if (!(element instanceof expectedType)) {
    throw new Error(`capability_catalog_element_missing: selector=${selector}`);
  }
  return element;
}

/**
 * @param {HTMLElement} element
 * @param {string} key
 * @returns {string}
 */
function requiredDatasetValue(element, key) {
  const value = element.dataset[key];
  if (!value) {
    throw new Error(`capability_catalog_data_missing: key=${key}`);
  }
  return value;
}

if (!customElements.get("capability-catalog")) {
  customElements.define("capability-catalog", CapabilityCatalogElement);
}
