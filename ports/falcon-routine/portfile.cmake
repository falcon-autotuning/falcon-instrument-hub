vcpkg_from_github(
    OUT_SOURCE_PATH SOURCE_PATH
    REPO falcon-autotuning/falcon-routine
    REF v${VERSION}
    SHA512 626f303219f757a8ca500b3d15a2115cf580c236ac8abb839b15c8b131f13ed3fe2ab882bb69e00f5c7264cce5fc1c9e46531d2413a24d6343ebee3bd1ba1373
)

# Inject local hub.cpp override (Phase 4b: inline MeasureResponse, no JetStream pull)
get_filename_component(WORKSPACE_ROOT "${CURRENT_PORT_DIR}/../../.." ABSOLUTE)
set(FALCON_ROUTINE_HUB_OVERRIDE "${WORKSPACE_ROOT}/falcon/comms/src/hub_override/hub.cpp")
if(EXISTS "${FALCON_ROUTINE_HUB_OVERRIDE}")
    file(COPY "${FALCON_ROUTINE_HUB_OVERRIDE}" DESTINATION "${SOURCE_PATH}/src")
endif()

vcpkg_cmake_configure(
    SOURCE_PATH "${SOURCE_PATH}"
)

vcpkg_cmake_install()
vcpkg_cmake_config_fixup()

file(INSTALL "${SOURCE_PATH}/LICENSE"
     DESTINATION "${CURRENT_PACKAGES_DIR}/share/${PORT}"
     RENAME copyright)

vcpkg_copy_pdbs()
