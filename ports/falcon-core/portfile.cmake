vcpkg_from_github(
    OUT_SOURCE_PATH SOURCE_PATH
    REPO falcon-autotuning/falcon-core
    REF v${VERSION}
    SHA512 dc1be4f16217d3e3b32497b75a9fefd9b4f6af6c462dfc484cf05b544434b02ecba502020c87f1caaebffc666f50be0fe9c9655bab35cfae62df56ac208392fb
)

set(BUILD_C_API OFF)
if("c-api" IN_LIST FEATURES)
  set(BUILD_C_API ON)
endif()

vcpkg_cmake_configure(
    SOURCE_PATH "${SOURCE_PATH}"
    OPTIONS
        -DFALCON_CORE_BUILD_C_API=${BUILD_C_API}
)

vcpkg_cmake_install()
vcpkg_cmake_config_fixup()

file(INSTALL "${SOURCE_PATH}/LICENSE.txt"
     DESTINATION "${CURRENT_PACKAGES_DIR}/share/${PORT}"
     RENAME copyright)

vcpkg_copy_pdbs()
