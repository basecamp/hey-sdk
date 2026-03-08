/*
 * Copyright Basecamp, LLC
 * SPDX-License-Identifier: Apache-2.0
 *
 * Transforms *ResponseContent schemas from wrapped objects to bare arrays.
 * This bridges the gap between Smithy's protocol constraints (which require
 * wrapped structures) and the HEY API's actual wire format (bare arrays).
 *
 * Applies to any response schema ending in ResponseContent that has exactly
 * one property which is an array type.
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
 * to bare arrays, matching the HEY API's actual response format.
 *
 * <p>Smithy's AWS restJson1 protocol requires list outputs to be modeled as
 * wrapped structures (e.g., {@code ListBoxesOutput { boxes: BoxList }})
 * because {@code @httpPayload} only supports structures, not arrays.
 *
 * <p>However, the HEY API returns bare arrays for list endpoints:
 * {@code GET /boxes.json} returns {@code [...]} not {@code {"boxes": [...]}}.
 */
public final class BareArrayResponseMapper implements OpenApiMapper {

    private static final Logger LOGGER = Logger.getLogger(BareArrayResponseMapper.class.getName());

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
                newSchemas.withMember(name, transformToArray(schema.expectObjectNode()));
                transformedCount++;
            } else {
                newSchemas.withMember(name, schema);
            }
        }

        if (transformedCount > 0) {
            LOGGER.info("Transformed " + transformedCount + " *ResponseContent schemas to bare arrays");
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

        return propValue.expectObjectNode()
                .getStringMember("type")
                .map(n -> n.getValue().equals("array"))
                .orElse(false);
    }

    private static final java.util.Set<String> STRUCTURAL_MEMBERS = java.util.Set.of(
            "type", "properties", "required", "items", "additionalProperties"
    );

    ObjectNode transformToArray(ObjectNode wrapped) {
        ObjectNode properties = wrapped.getObjectMember("properties").get();
        ObjectNode arrayProp = properties.getStringMap().values().iterator().next().expectObjectNode();

        ObjectNode.Builder result = ObjectNode.builder()
                .withMember("type", "array");

        arrayProp.getObjectMember("items").ifPresent(items ->
                result.withMember("items", items));

        copyNonStructuralMembers(arrayProp, result);
        copyNonStructuralMembersIfAbsent(wrapped, result, arrayProp);

        return result.build();
    }

    private void copyNonStructuralMembers(ObjectNode source, ObjectNode.Builder builder) {
        for (Map.Entry<String, Node> entry : source.getStringMap().entrySet()) {
            String key = entry.getKey();
            if (!STRUCTURAL_MEMBERS.contains(key)) {
                builder.withMember(key, entry.getValue());
            }
        }
    }

    private void copyNonStructuralMembersIfAbsent(ObjectNode source, ObjectNode.Builder builder, ObjectNode higherPriority) {
        for (Map.Entry<String, Node> entry : source.getStringMap().entrySet()) {
            String key = entry.getKey();
            if (!STRUCTURAL_MEMBERS.contains(key) && !higherPriority.getMember(key).isPresent()) {
                builder.withMember(key, entry.getValue());
            }
        }
    }
}
