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
  FAMILY: "[data-route-family]",
  MAP: "[data-route-map]",
  MODEL: "[data-route-model]",
  MODEL_GROUP: "[data-route-model-group]",
  PRODUCT: "[data-route-product]",
  PROVIDER: "[data-route-provider]",
  PROVIDER_GROUP: "[data-route-provider-group]",
  PROXY: "[data-route-proxy]",
  SELECTED_MODEL: "[data-route-selected-model]",
  SELECTED_PROVIDER: "[data-route-selected-provider]",
});

class RoutingTreeElement extends HTMLElement {
  constructor() {
    super();
    /** @type {HTMLButtonElement[]} */
    this.familyButtons = [];
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
    /** @type {HTMLElement | null} */
    this.selectedModelOutput = null;
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
    /** @type {ResizeObserver | null} */
    this.resizeObserver = null;
    this.drawFrameRequest = 0;
  }

  connectedCallback() {
    if (this.dataset.enhanced === SELECTED_ATTRIBUTE_VALUE) {
      return;
    }
    this.familyButtons = requiredButtons(this, SELECTORS.FAMILY);
    this.modelButtons = requiredButtons(this, SELECTORS.MODEL);
    this.modelGroups = routingGroups(this, SELECTORS.MODEL_GROUP, "routeModelGroup");
    this.providerGroups = routingGroups(this, SELECTORS.PROVIDER_GROUP, "routeProviderGroup");
    this.selectedModelOutput = requiredElement(this, SELECTORS.SELECTED_MODEL, HTMLElement);
    this.selectedProviderOutput = requiredElement(this, SELECTORS.SELECTED_PROVIDER, HTMLElement);
    this.routeMap = requiredElement(this, SELECTORS.MAP, HTMLElement);
    this.routeCanvas = requiredElement(this, SELECTORS.CANVAS, HTMLCanvasElement);
    this.productNode = requiredElement(this, SELECTORS.PRODUCT, HTMLElement);
    this.proxyNode = requiredElement(this, SELECTORS.PROXY, HTMLElement);

    if (this.familyButtons.length !== this.modelGroups.size || this.modelButtons.length !== this.providerGroups.size) {
      throw new Error(`routing_tree_normalized_group_count_invalid: families=${this.familyButtons.length} model_groups=${this.modelGroups.size} models=${this.modelButtons.length} provider_groups=${this.providerGroups.size}`);
    }
    const selectedFamilies = this.familyButtons.filter((button) => isSelected(button));
    const selectedModels = this.modelButtons.filter((button) => isSelected(button));
    if (selectedFamilies.length !== 1 || selectedModels.length !== 1) {
      throw new Error(`routing_tree_selection_invalid: families=${selectedFamilies.length} models=${selectedModels.length}`);
    }

    for (const familyButton of this.familyButtons) {
      familyButton.disabled = false;
      familyButton.addEventListener("click", () => this.selectFamily(familyButton));
    }
    for (const modelButton of this.modelButtons) {
      modelButton.disabled = false;
      modelButton.addEventListener("click", () => this.selectModel(modelButton));
    }
    for (const providerButton of requiredButtons(this, SELECTORS.PROVIDER)) {
      providerButton.disabled = false;
      providerButton.addEventListener("click", () => this.selectProvider(providerButton));
    }
    this.resizeObserver = new ResizeObserver(() => this.scheduleRouteDraw());
    this.resizeObserver.observe(this.routeMap);
    this.dataset.enhanced = SELECTED_ATTRIBUTE_VALUE;
    this.selectFamily(selectedFamilies[0]);
  }

  disconnectedCallback() {
    this.resizeObserver?.disconnect();
    if (this.drawFrameRequest !== 0) {
      cancelAnimationFrame(this.drawFrameRequest);
      this.drawFrameRequest = 0;
    }
  }

  /** @param {HTMLButtonElement} familyButton */
  selectFamily(familyButton) {
    const familyIdentifier = requiredDatasetValue(familyButton, "routeFamily");
    const modelGroup = this.modelGroups.get(familyIdentifier);
    if (!modelGroup) {
      throw new Error(`routing_tree_model_group_missing: family=${familyIdentifier}`);
    }
    for (const candidateFamily of this.familyButtons) {
      candidateFamily.setAttribute("aria-pressed", String(candidateFamily === familyButton));
    }
    for (const candidateModelGroup of this.modelGroups.values()) {
      candidateModelGroup.hidden = candidateModelGroup !== modelGroup;
    }
    this.activeModelGroup = modelGroup;
    const familyModels = requiredButtons(modelGroup, SELECTORS.MODEL);
    const selectedModel = familyModels.find((button) => isSelected(button)) ?? familyModels[0];
    this.selectModel(selectedModel);
  }

  /** @param {HTMLButtonElement} modelButton */
  selectModel(modelButton) {
    if (!this.activeModelGroup || !this.activeModelGroup.contains(modelButton)) {
      throw new Error("routing_tree_model_outside_active_family");
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
    if (!this.selectedModelOutput) {
      throw new Error("routing_tree_selected_model_output_missing");
    }
    this.selectedModelOutput.textContent = modelIdentifier;
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
    if (!this.routeMap || !this.routeCanvas || !this.productNode || !this.proxyNode || !this.activeModelGroup || !this.activeProviderGroup) {
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
    drawHorizontalCurve(drawingContext, productPoint, proxyPoint, accentColor, ACTIVE_LINE_WIDTH, ACTIVE_LINE_OPACITY);

    const familyConnections = this.familyButtons.map((familyButton) => ({
      active: isSelected(familyButton),
      point: relativeElementPoint(familyButton, mapBounds),
    }));
    for (const connection of familyConnections.sort((first, second) => Number(first.active) - Number(second.active))) {
      drawHorizontalCurve(
        drawingContext,
        proxyPoint,
        connection.point,
        connection.active ? accentColor : lineColor,
        connection.active ? ACTIVE_LINE_WIDTH : INACTIVE_LINE_WIDTH,
        connection.active ? ACTIVE_LINE_OPACITY : INACTIVE_LINE_OPACITY,
      );
    }
    const selectedFamily = this.familyButtons.find((familyButton) => isSelected(familyButton));
    if (!selectedFamily) {
      throw new Error("routing_tree_selected_family_missing");
    }
    const selectedFamilyPoint = relativeElementPoint(selectedFamily, mapBounds);
    const modelConnections = requiredButtons(this.activeModelGroup, SELECTORS.MODEL).map((modelButton) => ({
      active: isSelected(modelButton),
      point: relativeElementPoint(modelButton, mapBounds),
    }));
    for (const connection of modelConnections.sort((first, second) => Number(first.active) - Number(second.active))) {
      drawHorizontalCurve(
        drawingContext,
        selectedFamilyPoint,
        connection.point,
        connection.active ? accentColor : lineColor,
        connection.active ? ACTIVE_LINE_WIDTH : INACTIVE_LINE_WIDTH,
        connection.active ? ACTIVE_LINE_OPACITY : INACTIVE_LINE_OPACITY,
      );
    }
    const selectedModel = requiredButtons(this.activeModelGroup, SELECTORS.MODEL).find((modelButton) => isSelected(modelButton));
    if (!selectedModel) {
      throw new Error("routing_tree_selected_model_missing");
    }
    const selectedModelPoint = relativeElementPoint(selectedModel, mapBounds);
    const providerConnections = requiredButtons(this.activeProviderGroup, SELECTORS.PROVIDER).map((providerButton) => ({
      active: isSelected(providerButton),
      point: relativeElementPoint(providerButton, mapBounds),
    }));
    for (const connection of providerConnections.sort((first, second) => Number(first.active) - Number(second.active))) {
      drawHorizontalCurve(
        drawingContext,
        selectedModelPoint,
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
