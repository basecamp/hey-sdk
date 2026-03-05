/*
 * Copyright Basecamp, LLC
 * SPDX-License-Identifier: Apache-2.0
 *
 * Transforms *ResponseContent schemas from wrapped objects to bare $ref.
 * This bridges the gap between Smithy's protocol constraints (which require
 * wrapped structures) and the HEY API's actual wire format (bare objects).
 */
package com.basecamp.smithy;

import software.amazon.smithy.model.node.Node;
import software.amazon.smithy.model.node.ObjectNode;
import software.amazon.smithy.model.traits.Trait;
import software.amazon.smithy.openapi.fromsmithy.Context;
import software.amazon.smithy.openapi.fromsmithy.OpenApiMapper;
import software.amazon.smithy.openapi.model.OpenApi;

import java.util.Map;
import java.util.logging.Logger;

/**
 * An OpenAPI mapper that transforms response schemas from wrapped objects
 * to bare {@code $ref}, matching the HEY API's actual response format.
 *
 * <p>Transforms ALL {@code *ResponseContent} schemas that have exactly one
 * property with a {@code $ref}. Schemas with inline primitives (e.g.,
 * {@code { type: "string" }}) are NOT transformed to avoid losing the
 * property name.
 */
public final class BareObjectResponseMapper implements OpenApiMapper {

    private static final Logger LOGGER = Logger.getLogger(BareObjectResponseMapper.class.getName());

    @Override
    public byte getOrder() {
        return 100;
    }

    @Override
    public ObjectNode updateNode(Context<? extends Trait> context, OpenApi openapi, ObjectNode node) {
        ObjectNode componentsNode = node.getObjectMember("components").orElse(null);
        if (componentsNode == null) {
            return node;
        }

        ObjectNode schemasNode = componentsNode.getObjectMember("schemas").orElse(null);
        if (schemasNode == null) {
            return node;
        }

        ObjectNode.Builder newSchemas = ObjectNode.builder();
        int transformedCount = 0;

        for (Map.Entry<String, Node> entry : schemasNode.getStringMap().entrySet()) {
            String name = entry.getKey();
            Node schema = entry.getValue();

            if (shouldTransform(name, schema)) {
                newSchemas.withMember(name, transformToRef(schema.expectObjectNode()));
                transformedCount++;
            } else {
                newSchemas.withMember(name, schema);
            }
        }

        if (transformedCount > 0) {
            LOGGER.info("Transformed " + transformedCount + " *ResponseContent schemas to bare $ref");
        }

        ObjectNode newComponents = componentsNode.toBuilder()
                .withMember("schemas", newSchemas.build())
                .build();

        return node.toBuilder()
                .withMember("components", newComponents)
                .build();
    }

    boolean shouldTransform(String name, Node schema) {
        if (!name.endsWith("ResponseContent")) {
            return false;
        }

        if (!schema.isObjectNode()) {
            return false;
        }

        ObjectNode obj = schema.expectObjectNode();

        if (!obj.getStringMember("type").map(n -> n.getValue().equals("object")).orElse(false)) {
            return false;
        }

        ObjectNode properties = obj.getObjectMember("properties").orElse(null);
        if (properties == null) {
            return false;
        }

        Map<String, Node> props = properties.getStringMap();
        if (props.size() != 1) {
            return false;
        }

        Node propValue = props.values().iterator().next();
        if (!propValue.isObjectNode()) {
            return false;
        }

        ObjectNode propObj = propValue.expectObjectNode();
        return propObj.getMember("$ref").isPresent();
    }

    ObjectNode transformToRef(ObjectNode wrapped) {
        ObjectNode properties = wrapped.getObjectMember("properties").get();
        return properties.getStringMap().values().iterator().next().expectObjectNode();
    }
}
