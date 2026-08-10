// @ts-check

const ROUTING_TREE_ELEMENT_NAME = "routing-tree";
const SELECTED_ATTRIBUTE_VALUE = "true";
const MOBILE_LAYOUT_MAX_WIDTH = 680;
const DEVICE_PIXEL_RATIO_MINIMUM = 1;
const CONTROL_POINT_RATIO = 0.5;
const PROVIDER_CURVE_OFFSET = 28;
const INACTIVE_LINE_WIDTH = 1;
const ACTIVE_LINE_WIDTH = 2;
const INACTIVE_LINE_OPACITY = 0.42;
const MODEL_LINE_OPACITY = 0.5;
const ACTIVE_LINE_OPACITY = 1;

const SELECTORS = Object.freeze({
  CANVAS: "[data-route-canvas]",
  MAP: "[data-route-map]",
  MODEL: "[data-route-model]",
  MODEL_DEFAULT: '[data-route-default-model="true"]',
  MODEL_GROUP: "[data-route-model-group]",
  PRODUCT: "[data-route-product]",
  PROVIDER: "[data-route-provider]",
  PROXY: "[data-route-proxy]",
  SELECTED_MODEL: "[data-route-selected-model]",
  SELECTED_PROVIDER: "[data-route-selected-provider]",
});

class RoutingTreeElement extends HTMLElement {
  constructor() {
    super();
    /** @type {HTMLButtonElement[]} */
    this.providerButtons = [];
    /** @type {Map<string, HTMLElement>} */
    this.modelGroups = new Map();
    /** @type {HTMLElement | null} */
    this.activeModelGroup = null;
    /** @type {HTMLElement | null} */
    this.selectedProviderOutput = null;
    /** @type {HTMLElement | null} */
    this.selectedModelOutput = null;
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
    this.providerButtons = requiredButtons(this, SELECTORS.PROVIDER);
    this.modelGroups = routingModelGroups(this);
    if (this.providerButtons.length !== this.modelGroups.size) {
      throw new Error(`routing_tree_provider_group_count_invalid: providers=${this.providerButtons.length} groups=${this.modelGroups.size}`);
    }
    this.selectedProviderOutput = requiredElement(this, SELECTORS.SELECTED_PROVIDER, HTMLElement);
    this.selectedModelOutput = requiredElement(this, SELECTORS.SELECTED_MODEL, HTMLElement);
    this.routeMap = requiredElement(this, SELECTORS.MAP, HTMLElement);
    this.routeCanvas = requiredElement(this, SELECTORS.CANVAS, HTMLCanvasElement);
    this.productNode = requiredElement(this, SELECTORS.PRODUCT, HTMLElement);
    this.proxyNode = requiredElement(this, SELECTORS.PROXY, HTMLElement);

    const selectedProviderButtons = this.providerButtons.filter(
      (providerButton) => providerButton.getAttribute("aria-pressed") === SELECTED_ATTRIBUTE_VALUE,
    );
    if (selectedProviderButtons.length !== 1) {
      throw new Error(`routing_tree_selected_provider_invalid: count=${selectedProviderButtons.length}`);
    }

    for (const providerButton of this.providerButtons) {
      providerButton.disabled = false;
      providerButton.addEventListener("click", () => this.selectProvider(providerButton));
    }
    for (const modelButton of requiredButtons(this, SELECTORS.MODEL)) {
      modelButton.disabled = false;
      modelButton.addEventListener("click", () => this.selectModel(modelButton));
    }
    this.resizeObserver = new ResizeObserver(() => this.scheduleRouteDraw());
    this.resizeObserver.observe(this.routeMap);
    this.dataset.enhanced = SELECTED_ATTRIBUTE_VALUE;
    this.selectProvider(providerButtonWithMostModels(this.providerButtons, this.modelGroups));
  }

  disconnectedCallback() {
    this.resizeObserver?.disconnect();
    if (this.drawFrameRequest !== 0) {
      cancelAnimationFrame(this.drawFrameRequest);
      this.drawFrameRequest = 0;
    }
  }

  /** @param {HTMLButtonElement} providerButton */
  selectProvider(providerButton) {
    const providerIdentifier = requiredDatasetValue(providerButton, "routeProvider");
    const modelGroup = this.modelGroups.get(providerIdentifier);
    if (!modelGroup) {
      throw new Error(`routing_tree_model_group_missing: provider=${providerIdentifier}`);
    }
    for (const candidateProviderButton of this.providerButtons) {
      candidateProviderButton.setAttribute(
        "aria-pressed",
        String(candidateProviderButton === providerButton),
      );
    }
    for (const candidateModelGroup of this.modelGroups.values()) {
      candidateModelGroup.hidden = candidateModelGroup !== modelGroup;
    }
    this.activeModelGroup = modelGroup;
    if (!this.selectedProviderOutput) {
      throw new Error("routing_tree_provider_output_missing");
    }
    this.selectedProviderOutput.textContent = providerIdentifier;
    const defaultModelButton = requiredElement(modelGroup, SELECTORS.MODEL_DEFAULT, HTMLButtonElement);
    this.selectModel(defaultModelButton);
  }

  /** @param {HTMLButtonElement} modelButton */
  selectModel(modelButton) {
    if (!this.activeModelGroup || !this.activeModelGroup.contains(modelButton)) {
      throw new Error("routing_tree_model_outside_active_provider");
    }
    const modelIdentifier = requiredDatasetValue(modelButton, "routeModel");
    for (const candidateModelButton of requiredButtons(this.activeModelGroup, SELECTORS.MODEL)) {
      candidateModelButton.setAttribute(
        "aria-pressed",
        String(candidateModelButton === modelButton),
      );
    }
    if (!this.selectedModelOutput) {
      throw new Error("routing_tree_model_output_missing");
    }
    this.selectedModelOutput.textContent = modelIdentifier;
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
    if (!this.routeMap || !this.routeCanvas || !this.productNode || !this.proxyNode || !this.activeModelGroup) {
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
    drawHorizontalCurve(
      drawingContext,
      { x: productPoint.right, y: productPoint.y },
      { x: proxyPoint.left, y: proxyPoint.y },
      accentColor,
      ACTIVE_LINE_WIDTH,
      ACTIVE_LINE_OPACITY,
    );

    const providerConnections = this.providerButtons.map((providerButton) => ({
      active: providerButton.getAttribute("aria-pressed") === SELECTED_ATTRIBUTE_VALUE,
      point: relativeElementPoint(providerButton, mapBounds),
    }));
    const providersFollowProxy = providerConnections.every((connection) => connection.point.left >= proxyPoint.right);
    const providerStart = providersFollowProxy
      ? { x: proxyPoint.right, y: proxyPoint.y }
      : { x: proxyPoint.x, y: proxyPoint.bottom };
    for (const connection of providerConnections.sort((first, second) => Number(first.active) - Number(second.active))) {
      const drawProviderCurve = providersFollowProxy ? drawHorizontalCurve : drawForkCurve;
      drawProviderCurve(
        drawingContext,
        providerStart,
        { x: connection.point.left, y: connection.point.y },
        connection.active ? accentColor : lineColor,
        connection.active ? ACTIVE_LINE_WIDTH : INACTIVE_LINE_WIDTH,
        connection.active ? ACTIVE_LINE_OPACITY : INACTIVE_LINE_OPACITY,
      );
    }

    const selectedProviderButton = this.providerButtons.find(
      (providerButton) => providerButton.getAttribute("aria-pressed") === SELECTED_ATTRIBUTE_VALUE,
    );
    if (!selectedProviderButton) {
      throw new Error("routing_tree_selected_provider_missing");
    }
    const selectedProviderPoint = relativeElementPoint(selectedProviderButton, mapBounds);
    const modelConnections = requiredButtons(this.activeModelGroup, SELECTORS.MODEL).map((modelButton) => ({
      active: modelButton.getAttribute("aria-pressed") === SELECTED_ATTRIBUTE_VALUE,
      point: relativeElementPoint(modelButton, mapBounds),
    }));
    for (const connection of modelConnections.sort((first, second) => Number(first.active) - Number(second.active))) {
      drawHorizontalCurve(
        drawingContext,
        { x: selectedProviderPoint.right, y: selectedProviderPoint.y },
        { x: connection.point.left, y: connection.point.y },
        connection.active ? accentColor : lineColor,
        connection.active ? ACTIVE_LINE_WIDTH : INACTIVE_LINE_WIDTH,
        connection.active ? ACTIVE_LINE_OPACITY : MODEL_LINE_OPACITY,
      );
    }
    this.dataset.routeLinesRendered = SELECTED_ATTRIBUTE_VALUE;
  }
}

/**
 * @param {HTMLButtonElement[]} providerButtons
 * @param {Map<string, HTMLElement>} modelGroups
 */
function providerButtonWithMostModels(providerButtons, modelGroups) {
  let selectedProviderButton = providerButtons[0];
  let selectedModelCount = providerModelCount(selectedProviderButton, modelGroups);
  for (const providerButton of providerButtons.slice(1)) {
    const modelCount = providerModelCount(providerButton, modelGroups);
    if (modelCount > selectedModelCount) {
      selectedProviderButton = providerButton;
      selectedModelCount = modelCount;
    }
  }
  return selectedProviderButton;
}

/**
 * @param {HTMLButtonElement} providerButton
 * @param {Map<string, HTMLElement>} modelGroups
 */
function providerModelCount(providerButton, modelGroups) {
  const providerIdentifier = requiredDatasetValue(providerButton, "routeProvider");
  const modelGroup = modelGroups.get(providerIdentifier);
  if (!modelGroup) {
    throw new Error(`routing_tree_model_group_missing: provider=${providerIdentifier}`);
  }
  return modelGroup.querySelectorAll(SELECTORS.MODEL).length;
}

/**
 * @param {CanvasRenderingContext2D} drawingContext
 * @param {{x: number, y: number}} start
 * @param {{x: number, y: number}} end
 * @param {string} color
 * @param {number} width
 * @param {number} opacity
 */
function drawHorizontalCurve(drawingContext, start, end, color, width, opacity) {
  const horizontalDistance = (end.x - start.x) * CONTROL_POINT_RATIO;
  drawBezier(
    drawingContext,
    start,
    { x: start.x + horizontalDistance, y: start.y },
    { x: end.x - horizontalDistance, y: end.y },
    end,
    color,
    width,
    opacity,
  );
}

/**
 * @param {CanvasRenderingContext2D} drawingContext
 * @param {{x: number, y: number}} start
 * @param {{x: number, y: number}} end
 * @param {string} color
 * @param {number} width
 * @param {number} opacity
 */
function drawForkCurve(drawingContext, start, end, color, width, opacity) {
  drawBezier(
    drawingContext,
    start,
    { x: start.x, y: start.y + PROVIDER_CURVE_OFFSET },
    { x: end.x - PROVIDER_CURVE_OFFSET, y: end.y },
    end,
    color,
    width,
    opacity,
  );
}

/**
 * @param {CanvasRenderingContext2D} drawingContext
 * @param {{x: number, y: number}} start
 * @param {{x: number, y: number}} firstControl
 * @param {{x: number, y: number}} secondControl
 * @param {{x: number, y: number}} end
 * @param {string} color
 * @param {number} width
 * @param {number} opacity
 */
function drawBezier(drawingContext, start, firstControl, secondControl, end, color, width, opacity) {
  drawingContext.beginPath();
  drawingContext.moveTo(start.x, start.y);
  drawingContext.bezierCurveTo(firstControl.x, firstControl.y, secondControl.x, secondControl.y, end.x, end.y);
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
  const left = rectangle.left - bounds.left;
  const top = rectangle.top - bounds.top;
  return {
    bottom: rectangle.bottom - bounds.top,
    left,
    right: rectangle.right - bounds.left,
    top,
    x: left + rectangle.width * CONTROL_POINT_RATIO,
    y: top + rectangle.height * CONTROL_POINT_RATIO,
  };
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
 * @returns {Map<string, HTMLElement>}
 */
function routingModelGroups(root) {
  const modelGroups = new Map();
  for (const element of root.querySelectorAll(SELECTORS.MODEL_GROUP)) {
    if (!(element instanceof HTMLElement)) {
      throw new Error("routing_tree_model_group_invalid");
    }
    const providerIdentifier = requiredDatasetValue(element, "routeModelGroup");
    if (modelGroups.has(providerIdentifier)) {
      throw new Error(`routing_tree_model_group_duplicate: provider=${providerIdentifier}`);
    }
    modelGroups.set(providerIdentifier, element);
  }
  return modelGroups;
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
