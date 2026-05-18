vcpkg_from_github(
    OUT_SOURCE_PATH SOURCE_PATH
    REPO falcon-autotuning/falcon-comms
    REF v${VERSION}
    SHA512 51e2315de2eaa37b0fafe8ccd93b60393945715737ac661c6e64cd4380a56cc03b952e8143cb4d4e676e556e5727132166398c9ef289f3954a0fb28b3359879f
)

# Inject local routine_comms.cpp override (Phase 4b: inline MeasureResponse, no JetStream)
get_filename_component(WORKSPACE_ROOT "${CURRENT_PORT_DIR}/../../.." ABSOLUTE)
set(FALCON_COMMS_OVERRIDE "${WORKSPACE_ROOT}/falcon/comms/src/routine_comms.cpp")
if(EXISTS "${FALCON_COMMS_OVERRIDE}")
    file(COPY "${FALCON_COMMS_OVERRIDE}" DESTINATION "${SOURCE_PATH}/src")
endif()
# Inject local header to keep declaration in sync with the implementation override
set(FALCON_COMMS_HDR_OVERRIDE "${WORKSPACE_ROOT}/falcon/comms/include/falcon-comms/routine_comms.hpp")
if(EXISTS "${FALCON_COMMS_HDR_OVERRIDE}")
    file(COPY "${FALCON_COMMS_HDR_OVERRIDE}" DESTINATION "${SOURCE_PATH}/include/falcon-comms")
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
