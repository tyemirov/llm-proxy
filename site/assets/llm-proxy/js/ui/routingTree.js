// @ts-check

const ROUTING_TREE_ELEMENT_NAME = "routing-tree";
const SELECTED_ATTRIBUTE_VALUE = "true";
const MOBILE_LAYOUT_MAX_WIDTH = 680;
const DEVICE_PIXEL_RATIO_MINIMUM = 1;
const CONTROL_POINT_RATIO = 0.5;
const INACTIVE_LINE_WIDTH = 1;
const ACTIVE_LINE_WIDTH = 2;
const INACTIVE_LINE_OPACITY = 0.42;
const ACTIVE_LINE_OPACITY = 1;

const SELECTORS = Object.freeze({
  CANVAS: "[data-route-canvas]",
  EMPTY: "[data-route-empty]",
  FAMILY_FILTER: "[data-route-family-filter]",
  MAP: "[data-route-map]",
  MODEL: "[data-route-model]",
  MODEL_GROUP: "[data-route-model-group]",
  OPERATION_FILTER: "[data-route-operation-filter]",
  PICKER: "[data-route-picker]",
  PRODUCT: "[data-route-product]",
  PROVIDER: "[data-route-provider]",
  PROVIDER_GROUP: "[data-route-provider-group]",
  PROXY: "[data-route-proxy]",
  PUBLISHER: "[data-route-publisher]",
  RESET: "[data-route-reset]",
  SEARCH: "[data-route-search]",
  SELECTED_MODEL: "[data-route-selected-model]",
  SELECTED_PROVIDER: "[data-route-selected-provider]",
  SELECTED_PUBLISHER: "[data-route-selected-publisher]",
  SELECTED_ROUTE_MODEL: "[data-route-selected-route-model]",
  SELECTION_NODE: "[data-route-selection-node]",
});

class RoutingTreeElement extends HTMLElement {
  constructor() {
    super();
    /** @type {HTMLButtonElement[]} */
    this.publisherButtons = [];
    /** @type {HTMLButtonElement[]} */
    this.modelButtons = [];
    /** @type {Map<string, HTMLElement>} */
    this.modelGroups = new Map();
    /** @type {Map<string, HTMLElement>} */
    this.providerGroups = new Map();
    /** @type {HTMLElement | null} */
    this.activeModelGroup = null;
    /** @type {HTMLElement | null} */
    this.activeProviderGroup = null;
    /** @type {HTMLFormElement | null} */
    this.picker = null;
    /** @type {HTMLInputElement | null} */
    this.searchInput = null;
    /** @type {HTMLSelectElement | null} */
    this.familyFilter = null;
    /** @type {HTMLSelectElement | null} */
    this.operationFilter = null;
    /** @type {HTMLElement | null} */
    this.emptyState = null;
    /** @type {HTMLElement | null} */
    this.selectedPublisherOutput = null;
    /** @type {HTMLElement | null} */
    this.selectedModelOutput = null;
    /** @type {HTMLElement | null} */
    this.selectedRouteModelOutput = null;
    /** @type {HTMLElement | null} */
    this.selectedProviderOutput = null;
    /** @type {HTMLElement | null} */
    this.routeMap = null;
    /** @type {HTMLCanvasElement | null} */
    this.routeCanvas = null;
    /** @type {HTMLElement | null} */
    this.productNode = null;
    /** @type {HTMLElement | null} */
    this.proxyNode = null;
    /** @type {HTMLElement | null} */
    this.selectionNode = null;
    /** @type {ResizeObserver | null} */
    this.resizeObserver = null;
    this.drawFrameRequest = 0;
  }

  connectedCallback() {
    if (this.dataset.enhanced === SELECTED_ATTRIBUTE_VALUE) {
      return;
    }
    this.publisherButtons = requiredButtons(this, SELECTORS.PUBLISHER);
    this.modelButtons = requiredButtons(this, SELECTORS.MODEL);
    this.modelGroups = routingGroups(this, SELECTORS.MODEL_GROUP, "routeModelGroup");
    this.providerGroups = routingGroups(this, SELECTORS.PROVIDER_GROUP, "routeProviderGroup");
    this.picker = requiredElement(this, SELECTORS.PICKER, HTMLFormElement);
    this.searchInput = requiredElement(this, SELECTORS.SEARCH, HTMLInputElement);
    this.familyFilter = requiredElement(this, SELECTORS.FAMILY_FILTER, HTMLSelectElement);
    this.operationFilter = requiredElement(this, SELECTORS.OPERATION_FILTER, HTMLSelectElement);
    this.emptyState = requiredElement(this, SELECTORS.EMPTY, HTMLElement);
    this.selectedPublisherOutput = requiredElement(this, SELECTORS.SELECTED_PUBLISHER, HTMLElement);
    this.selectedModelOutput = requiredElement(this, SELECTORS.SELECTED_MODEL, HTMLElement);
    this.selectedRouteModelOutput = requiredElement(this, SELECTORS.SELECTED_ROUTE_MODEL, HTMLElement);
    this.selectedProviderOutput = requiredElement(this, SELECTORS.SELECTED_PROVIDER, HTMLElement);
    this.routeMap = requiredElement(this, SELECTORS.MAP, HTMLElement);
    this.routeCanvas = requiredElement(this, SELECTORS.CANVAS, HTMLCanvasElement);
    this.productNode = requiredElement(this, SELECTORS.PRODUCT, HTMLElement);
    this.proxyNode = requiredElement(this, SELECTORS.PROXY, HTMLElement);
    this.selectionNode = requiredElement(this, SELECTORS.SELECTION_NODE, HTMLElement);

    if (this.publisherButtons.length !== this.modelGroups.size || this.modelButtons.length !== this.providerGroups.size) {
      throw new Error(`routing_tree_normalized_group_count_invalid: publishers=${this.publisherButtons.length} model_groups=${this.modelGroups.size} models=${this.modelButtons.length} provider_groups=${this.providerGroups.size}`);
    }
    const selectedPublishers = this.publisherButtons.filter((button) => isSelected(button));
    const selectedModels = this.modelButtons.filter((button) => isSelected(button));
    if (selectedPublishers.length !== 1 || selectedModels.length !== 1) {
      throw new Error(`routing_tree_selection_invalid: publishers=${selectedPublishers.length} models=${selectedModels.length}`);
    }

    for (const control of this.picker.elements) {
      if (control instanceof HTMLInputElement || control instanceof HTMLSelectElement || control instanceof HTMLButtonElement) {
        control.disabled = false;
      }
    }
    for (const publisherButton of this.publisherButtons) {
      publisherButton.disabled = false;
      publisherButton.addEventListener("click", () => this.selectPublisher(publisherButton));
    }
    for (const modelButton of this.modelButtons) {
      modelButton.disabled = false;
      modelButton.addEventListener("click", () => this.selectModel(modelButton));
    }
    for (const providerButton of requiredButtons(this, SELECTORS.PROVIDER)) {
      providerButton.disabled = false;
      providerButton.addEventListener("click", () => this.selectProvider(providerButton));
    }
    this.picker.addEventListener("input", () => this.applyFilters());
    this.picker.addEventListener("change", () => this.applyFilters());
    this.picker.addEventListener("reset", () => requestAnimationFrame(() => this.applyFilters()));
    this.resizeObserver = new ResizeObserver(() => this.scheduleRouteDraw());
    this.resizeObserver.observe(this.routeMap);
    this.dataset.enhanced = SELECTED_ATTRIBUTE_VALUE;
    this.applyFilters();
  }

  disconnectedCallback() {
    this.resizeObserver?.disconnect();
    if (this.drawFrameRequest !== 0) {
      cancelAnimationFrame(this.drawFrameRequest);
      this.drawFrameRequest = 0;
    }
  }

  applyFilters() {
    if (!this.searchInput || !this.familyFilter || !this.operationFilter || !this.emptyState) {
      throw new Error("routing_tree_picker_not_connected");
    }
    const searchTerms = normalizedSearchTerms(this.searchInput.value);
    const selectedFamily = this.familyFilter.value;
    const selectedOperation = this.operationFilter.value;
    for (const modelButton of this.modelButtons) {
      const family = requiredDatasetValue(modelButton, "routeFamily");
      const operations = requiredDatasetValue(modelButton, "routeOperations").split(" ");
      const searchText = requiredDatasetValue(modelButton, "routeSearchText").toLocaleLowerCase();
      modelButton.hidden = !searchTerms.every((term) => searchText.includes(term)) ||
        (selectedFamily !== "" && family !== selectedFamily) ||
        (selectedOperation !== "" && !operations.includes(selectedOperation));
    }
    for (const publisherButton of this.publisherButtons) {
      const publisherIdentifier = requiredDatasetValue(publisherButton, "routePublisher");
      const modelGroup = this.modelGroups.get(publisherIdentifier);
      if (!modelGroup) {
        throw new Error(`routing_tree_model_group_missing: publisher=${publisherIdentifier}`);
      }
      publisherButton.hidden = visibleButtons(modelGroup, SELECTORS.MODEL).length === 0;
    }
    const visiblePublishers = this.publisherButtons.filter((button) => !button.hidden);
    this.emptyState.hidden = visiblePublishers.length !== 0;
    if (visiblePublishers.length === 0) {
      for (const modelGroup of this.modelGroups.values()) {
        modelGroup.hidden = true;
      }
      this.scheduleRouteDraw();
      return;
    }
    const selectedPublisher = this.publisherButtons.find((button) => isSelected(button));
    if (!selectedPublisher || selectedPublisher.hidden) {
      this.selectPublisher(visiblePublishers[0]);
      return;
    }
    const publisherIdentifier = requiredDatasetValue(selectedPublisher, "routePublisher");
    const selectedModelGroup = this.modelGroups.get(publisherIdentifier);
    if (!selectedModelGroup) {
      throw new Error(`routing_tree_model_group_missing: publisher=${publisherIdentifier}`);
    }
    if (this.activeModelGroup !== selectedModelGroup) {
      this.selectPublisher(selectedPublisher);
      return;
    }
    const selectedModel = this.modelButtons.find((button) => isSelected(button));
    if (!selectedModel || selectedModel.hidden || requiredDatasetValue(selectedModel, "routeModelPublisher") !== publisherIdentifier) {
      this.selectPublisher(selectedPublisher);
      return;
    }
    this.selectModel(selectedModel);
  }

  /** @param {HTMLButtonElement} publisherButton */
  selectPublisher(publisherButton) {
    const publisherIdentifier = requiredDatasetValue(publisherButton, "routePublisher");
    const modelGroup = this.modelGroups.get(publisherIdentifier);
    if (!modelGroup) {
      throw new Error(`routing_tree_model_group_missing: publisher=${publisherIdentifier}`);
    }
    const selectableModels = visibleButtons(modelGroup, SELECTORS.MODEL);
    if (selectableModels.length === 0) {
      throw new Error(`routing_tree_publisher_without_visible_models: publisher=${publisherIdentifier}`);
    }
    for (const candidatePublisher of this.publisherButtons) {
      candidatePublisher.setAttribute("aria-pressed", String(candidatePublisher === publisherButton));
    }
    for (const candidateModelGroup of this.modelGroups.values()) {
      candidateModelGroup.hidden = candidateModelGroup !== modelGroup;
    }
    this.activeModelGroup = modelGroup;
    const selectedModel = selectableModels.find((button) => isSelected(button)) ?? selectableModels[0];
    this.selectModel(selectedModel);
  }

  /** @param {HTMLButtonElement} modelButton */
  selectModel(modelButton) {
    if (!this.activeModelGroup || !this.activeModelGroup.contains(modelButton) || modelButton.hidden) {
      throw new Error("routing_tree_model_outside_active_publisher");
    }
    const modelIdentifier = requiredDatasetValue(modelButton, "routeModel");
    const providerGroup = this.providerGroups.get(modelIdentifier);
    if (!providerGroup) {
      throw new Error(`routing_tree_provider_group_missing: model=${modelIdentifier}`);
    }
    for (const candidateModel of this.modelButtons) {
      candidateModel.setAttribute("aria-pressed", String(candidateModel === modelButton));
    }
    for (const candidateProviderGroup of this.providerGroups.values()) {
      candidateProviderGroup.hidden = candidateProviderGroup !== providerGroup;
    }
    this.activeProviderGroup = providerGroup;
    const selectedPublisher = this.publisherButtons.find((button) => isSelected(button));
    if (!selectedPublisher || !this.selectedPublisherOutput || !this.selectedModelOutput || !this.selectedRouteModelOutput) {
      throw new Error("routing_tree_selected_model_output_missing");
    }
    this.selectedPublisherOutput.textContent = requiredButtonLabel(selectedPublisher);
    this.selectedModelOutput.textContent = modelIdentifier;
    this.selectedRouteModelOutput.textContent = modelIdentifier;
    const providerButtons = requiredButtons(providerGroup, SELECTORS.PROVIDER);
    this.selectProvider(providerButtons.find((button) => isSelected(button)) ?? providerButtons[0]);
  }

  /** @param {HTMLButtonElement} providerButton */
  selectProvider(providerButton) {
    if (!this.activeProviderGroup || !this.activeProviderGroup.contains(providerButton)) {
      throw new Error("routing_tree_provider_outside_active_model");
    }
    const providerIdentifier = requiredDatasetValue(providerButton, "routeProvider");
    for (const candidateProvider of requiredButtons(this, SELECTORS.PROVIDER)) {
      candidateProvider.setAttribute("aria-pressed", String(candidateProvider === providerButton));
    }
    if (!this.selectedProviderOutput) {
      throw new Error("routing_tree_provider_output_missing");
    }
    this.selectedProviderOutput.textContent = providerIdentifier;
    this.scheduleRouteDraw();
  }

  scheduleRouteDraw() {
    if (this.drawFrameRequest !== 0) {
      return;
    }
    this.drawFrameRequest = requestAnimationFrame(() => {
      this.drawFrameRequest = 0;
      this.drawRoutes();
    });
  }

  drawRoutes() {
    if (!this.routeMap || !this.routeCanvas || !this.productNode || !this.proxyNode || !this.selectionNode || !this.activeProviderGroup) {
      throw new Error("routing_tree_drawing_surface_missing");
    }
    const mapBounds = this.routeMap.getBoundingClientRect();
    if (window.innerWidth <= MOBILE_LAYOUT_MAX_WIDTH) {
      this.dataset.routeLinesRendered = "false";
      return;
    }
    const drawingContext = this.routeCanvas.getContext("2d");
    if (!drawingContext) {
      throw new Error("routing_tree_canvas_context_missing");
    }
    const pixelRatio = Math.max(DEVICE_PIXEL_RATIO_MINIMUM, window.devicePixelRatio);
    const canvasWidth = Math.round(mapBounds.width);
    const canvasHeight = Math.round(mapBounds.height);
    this.routeCanvas.width = Math.round(canvasWidth * pixelRatio);
    this.routeCanvas.height = Math.round(canvasHeight * pixelRatio);
    drawingContext.setTransform(pixelRatio, 0, 0, pixelRatio, 0, 0);
    drawingContext.clearRect(0, 0, canvasWidth, canvasHeight);
    drawingContext.lineCap = "round";

    const treeStyles = getComputedStyle(this);
    const lineColor = requiredCSSProperty(treeStyles, "--routing-tree-line");
    const accentColor = requiredCSSProperty(treeStyles, "--routing-tree-accent");
    const productPoint = relativeElementPoint(this.productNode, mapBounds);
    const proxyPoint = relativeElementPoint(this.proxyNode, mapBounds);
    const selectionPoint = relativeElementPoint(this.selectionNode, mapBounds);
    drawHorizontalCurve(drawingContext, productPoint, proxyPoint, accentColor, ACTIVE_LINE_WIDTH, ACTIVE_LINE_OPACITY);
    drawHorizontalCurve(drawingContext, proxyPoint, selectionPoint, accentColor, ACTIVE_LINE_WIDTH, ACTIVE_LINE_OPACITY);
    const providerConnections = requiredButtons(this.activeProviderGroup, SELECTORS.PROVIDER).map((providerButton) => ({
      active: isSelected(providerButton),
      point: relativeElementPoint(providerButton, mapBounds),
    }));
    for (const connection of providerConnections.sort((first, second) => Number(first.active) - Number(second.active))) {
      drawHorizontalCurve(
        drawingContext,
        selectionPoint,
        connection.point,
        connection.active ? accentColor : lineColor,
        connection.active ? ACTIVE_LINE_WIDTH : INACTIVE_LINE_WIDTH,
        connection.active ? ACTIVE_LINE_OPACITY : INACTIVE_LINE_OPACITY,
      );
    }
    this.dataset.routeLinesRendered = SELECTED_ATTRIBUTE_VALUE;
  }
}

/**
 * @param {CanvasRenderingContext2D} drawingContext
 * @param {{left: number, right: number, y: number}} startPoint
 * @param {{left: number, right: number, y: number}} endPoint
 * @param {string} color
 * @param {number} width
 * @param {number} opacity
 */
function drawHorizontalCurve(drawingContext, startPoint, endPoint, color, width, opacity) {
  const start = { x: startPoint.right, y: startPoint.y };
  const end = { x: endPoint.left, y: endPoint.y };
  const horizontalDistance = (end.x - start.x) * CONTROL_POINT_RATIO;
  drawingContext.beginPath();
  drawingContext.moveTo(start.x, start.y);
  drawingContext.bezierCurveTo(
    start.x + horizontalDistance,
    start.y,
    end.x - horizontalDistance,
    end.y,
    end.x,
    end.y,
  );
  drawingContext.strokeStyle = color;
  drawingContext.lineWidth = width;
  drawingContext.globalAlpha = opacity;
  drawingContext.stroke();
  drawingContext.globalAlpha = ACTIVE_LINE_OPACITY;
}

/**
 * @param {HTMLElement} element
 * @param {DOMRect} bounds
 */
function relativeElementPoint(element, bounds) {
  const rectangle = element.getBoundingClientRect();
  return {
    left: rectangle.left - bounds.left,
    right: rectangle.right - bounds.left,
    y: rectangle.top - bounds.top + rectangle.height * CONTROL_POINT_RATIO,
  };
}

/** @param {HTMLButtonElement} button */
function isSelected(button) {
  return button.getAttribute("aria-pressed") === SELECTED_ATTRIBUTE_VALUE;
}

/**
 * @param {ParentNode} root
 * @param {string} selector
 */
function visibleButtons(root, selector) {
  return requiredButtons(root, selector).filter((button) => !button.hidden);
}

/** @param {string} rawSearch */
function normalizedSearchTerms(rawSearch) {
  return rawSearch.trim().toLocaleLowerCase().split(/\s+/u).filter(Boolean);
}

/** @param {HTMLButtonElement} button */
function requiredButtonLabel(button) {
  const label = button.querySelector("strong")?.textContent?.trim();
  if (!label) {
    throw new Error("routing_tree_button_label_missing");
  }
  return label;
}

/**
 * @param {CSSStyleDeclaration} styles
 * @param {string} propertyName
 */
function requiredCSSProperty(styles, propertyName) {
  const propertyValue = styles.getPropertyValue(propertyName).trim();
  if (!propertyValue) {
    throw new Error(`routing_tree_css_property_missing: property=${propertyName}`);
  }
  return propertyValue;
}

/**
 * @param {ParentNode} root
 * @param {string} selector
 * @param {string} datasetKey
 * @returns {Map<string, HTMLElement>}
 */
function routingGroups(root, selector, datasetKey) {
  const groups = new Map();
  for (const element of root.querySelectorAll(selector)) {
    if (!(element instanceof HTMLElement)) {
      throw new Error(`routing_tree_group_invalid: selector=${selector}`);
    }
    const identifier = requiredDatasetValue(element, datasetKey);
    if (groups.has(identifier)) {
      throw new Error(`routing_tree_group_duplicate: selector=${selector} identifier=${identifier}`);
    }
    groups.set(identifier, element);
  }
  return groups;
}

/**
 * @param {ParentNode} root
 * @param {string} selector
 * @returns {HTMLButtonElement[]}
 */
function requiredButtons(root, selector) {
  const buttons = [...root.querySelectorAll(selector)].map((element) => {
    if (!(element instanceof HTMLButtonElement)) {
      throw new Error(`routing_tree_button_invalid: selector=${selector}`);
    }
    return element;
  });
  if (buttons.length === 0) {
    throw new Error(`routing_tree_buttons_missing: selector=${selector}`);
  }
  return buttons;
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
    throw new Error(`routing_tree_element_missing: selector=${selector}`);
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
    throw new Error(`routing_tree_data_missing: key=${key}`);
  }
  return value;
}

if (!customElements.get(ROUTING_TREE_ELEMENT_NAME)) {
  customElements.define(ROUTING_TREE_ELEMENT_NAME, RoutingTreeElement);
}
